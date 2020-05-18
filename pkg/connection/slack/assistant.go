/*
2019 © Postgres.ai
*/

// Package slack provides the Slack implementation of the communication interface.
package slack

import (
	"context"
	"fmt"
	slog "log"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"

	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/features"
	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/models"
	"gitlab.com/postgres-ai/joe/pkg/services/dblab"
	"gitlab.com/postgres-ai/joe/pkg/services/msgproc"
	"gitlab.com/postgres-ai/joe/pkg/services/platform"
	"gitlab.com/postgres-ai/joe/pkg/services/usermanager"
)

var (
	linkRegexp  = regexp.MustCompile(`<http:\/\/[\w.]+\|([.\w]+)>`)
	emailRegexp = regexp.MustCompile(`<mailto:['@\w.]+\|(['@.\w]+)>`)
)

// CommunicationType defines a workspace type.
const CommunicationType = "slack"

// Assistant provides a service for interaction with a communication channel.
type Assistant struct {
	credentialsCfg  *config.Credentials
	procMu          sync.RWMutex
	msgProcessors   map[string]connection.MessageProcessor
	appCfg          *config.Config
	featurePack     *features.Pack
	rtm             *slack.RTM
	messenger       *Messenger
	userManager     *usermanager.UserManager
	platformManager *platform.Client
}

// SlackConfig defines a slack configuration parameters.
type SlackConfig struct {
	AccessToken   string
	SigningSecret string
}

// NewAssistant returns a new assistant service.
func NewAssistant(cfg *config.Credentials, appCfg *config.Config, pack *features.Pack) (*Assistant, error) {
	slackCfg := &SlackConfig{
		AccessToken:   cfg.AccessToken,
		SigningSecret: cfg.SigningSecret,
	}

	chatAPI := slack.New(slackCfg.AccessToken,
		slack.OptionDebug(true),
		slack.OptionLog(slog.New(os.Stdout, "slack-bot: ", slog.Lshortfile|slog.LstdFlags)),
	)

	rtm := chatAPI.NewRTM()

	messenger := NewMessenger(rtm, slackCfg)
	userInformer := NewUserInformer(rtm)
	userManager := usermanager.NewUserManager(userInformer, appCfg.Enterprise.Quota)

	platformManager, err := platform.NewClient(appCfg.Platform)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a Platform client")
	}

	assistant := &Assistant{
		credentialsCfg:  cfg,
		appCfg:          appCfg,
		msgProcessors:   make(map[string]connection.MessageProcessor),
		featurePack:     pack,
		rtm:             rtm,
		messenger:       messenger,
		userManager:     userManager,
		platformManager: platformManager,
	}

	return assistant, nil
}

func (a *Assistant) validateCredentials() error {
	if a.credentialsCfg == nil || a.credentialsCfg.AccessToken == "" || a.credentialsCfg.SigningSecret == "" {
		return errors.New(`"accessToken" and "signingSecret" must not be empty`)
	}

	return nil
}

// Init registers assistant handlers.
func (a *Assistant) Init(ctx context.Context) error {
	log.Dbg("Init Slack")

	if err := a.validateCredentials(); err != nil {
		return errors.Wrap(err, "invalid credentials given")
	}

	if a.lenMessageProcessor() == 0 {
		return errors.New("no message processor set")
	}

	go a.rtm.ManageConnection()

	go a.handleRTM(ctx, a.rtm.IncomingEvents)

	return nil
}

// AddDBLabInstanceForChannel sets a message processor for a specific channel.
func (a *Assistant) AddDBLabInstanceForChannel(channelID string, dbLabInstance *dblab.Instance) error {
	messageProcessor := a.buildMessageProcessor(dbLabInstance)

	a.addProcessingService(channelID, messageProcessor)

	return nil
}

func (a *Assistant) buildMessageProcessor(dbLabInstance *dblab.Instance) *msgproc.ProcessingService {
	processingCfg := msgproc.ProcessingConfig{
		App:      a.appCfg.App,
		Platform: a.appCfg.Platform,
		Explain:  a.appCfg.Explain,
		DBLab:    dbLabInstance.Config(),
		EntOpts:  a.appCfg.Enterprise,
	}

	return msgproc.NewProcessingService(a.messenger, MessageValidator{}, dbLabInstance.Client(), a.userManager, a.platformManager,
		processingCfg, a.featurePack)
}

func (a *Assistant) handleRTM(ctx context.Context, incomingEvents chan slack.RTMEvent) {
	for msg := range incomingEvents {
		select {
		case <-ctx.Done():
			return
		default:
		}

		switch ev := msg.Data.(type) {
		case *slack.MessageEvent:
			log.Dbg("Event type: Message")

			if ev.Msg.SubType != "" {
				// Handle only normal messages.
				continue
			}

			if ev.BotID != "" {
				// Skip messages sent by bots.
				continue
			}

			msgProcessor, err := a.getProcessingService(ev.Channel)
			if err != nil {
				log.Err("failed to get processing service", err)
				continue
			}

			msg := a.messageEventToIncomingMessage(ev)
			msgProcessor.ProcessMessageEvent(context.TODO(), msg)

		case *slack.DisconnectedEvent:
			fmt.Printf("Disconnect event: %v\n", ev.Cause.Error())

		case *slack.LatencyReport:
			fmt.Printf("Current latency: %v\n", ev.Value)

		default:
			log.Dbg(fmt.Sprintf("Event filtered: skip %q event type", msg.Type))
		}
	}
}

// addProcessingService adds a message processor for a specific channel.
func (a *Assistant) addProcessingService(channelID string, messageProcessor connection.MessageProcessor) {
	a.procMu.Lock()
	a.msgProcessors[channelID] = messageProcessor
	a.procMu.Unlock()
}

// getProcessingService returns processing service by channelID.
func (a *Assistant) getProcessingService(channelID string) (connection.MessageProcessor, error) {
	a.procMu.RLock()
	defer a.procMu.RUnlock()

	messageProcessor, ok := a.msgProcessors[channelID]
	if !ok {
		return nil, errors.Errorf("message processor for %q channel not found", channelID)
	}

	return messageProcessor, nil
}

// CheckIdleSessions check the running user sessions for idleness.
func (a *Assistant) CheckIdleSessions(ctx context.Context) {
	log.Dbg("Check Slack idle sessions")

	a.procMu.RLock()
	for _, proc := range a.msgProcessors {
		proc.CheckIdleSessions(ctx)
	}
	a.procMu.RUnlock()
}

func (a *Assistant) lenMessageProcessor() int {
	a.procMu.RLock()
	defer a.procMu.RUnlock()

	return len(a.msgProcessors)
}

/*func (a *Assistant) handlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"": a.handleEvent,
	}
}*/

/*func (a *Assistant) handleEvent(w http.ResponseWriter, r *http.Request) {
	log.Msg("Request received:", html.EscapeString(r.URL.Path))

	// TODO(anatoly): Respond time according to Slack API timeouts policy.
	// Slack sends retries in case of timeout responses.
	if r.Header.Get("X-Slack-Retry-Num") != "" {
		log.Dbg("Message filtered: Slack Retry")
		return
	}

	if err := a.verifyRequest(r); err != nil {
		log.Dbg("Message filtered: Verification failed:", err.Error())
		w.WriteHeader(http.StatusForbidden)

		return
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r.Body); err != nil {
		log.Err("Failed to read the request body:", err)
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	body := buf.Bytes()

	eventsAPIEvent, err := a.parseEvent(body)
	if err != nil {
		log.Err("Event parse error:", err)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	// TODO (akartasov): event processing function.
	switch eventsAPIEvent.Type {
	// Used to verify bot's API URL for Slack.
	case slackevents.URLVerification:
		log.Dbg("Event type: URL verification")

		var r *slackevents.ChallengeResponse

		err := json.Unmarshal(body, &r)
		if err != nil {
			log.Err("Challenge parse error:", err)
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "text")
		w.Write([]byte(r.Challenge))

	// General Slack events.
	case slackevents.CallbackEvent:
		switch ev := eventsAPIEvent.InnerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			log.Dbg("Event type: AppMention")

			msgProcessor, err := a.getProcessingService(ev.Channel)
			if err != nil {
				log.Err("failed to get processing service", err)
				return
			}

			msg := a.appMentionEventToIncomingMessage(ev)
			msgProcessor.ProcessAppMentionEvent(msg)

		case *slackevents.MessageEvent:
			log.Dbg("Event type: Message")

			if ev.BotID != "" {
				// Skip messages sent by bots.
				return
			}

			msgProcessor, err := a.getProcessingService(ev.Channel)
			if err != nil {
				log.Err("failed to get processing service", err)
				return
			}

			msg := a.messageEventToIncomingMessage(ev)
			msgProcessor.ProcessMessageEvent(context.TODO(), msg)

		default:
			log.Dbg("Event filtered: Inner event type not supported")
		}

	default:
		log.Dbg("Event filtered: Event type not supported")
	}
}*/

// appMentionEventToIncomingMessage converts a Slack application mention event to the standard incoming message.
func (a *Assistant) appMentionEventToIncomingMessage(event *slackevents.AppMentionEvent) models.IncomingMessage {
	inputEvent := models.IncomingMessage{
		Text:      event.Text,
		ChannelID: event.Channel,
		UserID:    event.User,
		Timestamp: event.TimeStamp,
		ThreadID:  event.ThreadTimeStamp,
	}

	return inputEvent
}

// messageEventToIncomingMessage converts a Slack message event to the standard incoming message.
func (a *Assistant) messageEventToIncomingMessage(event *slack.MessageEvent) models.IncomingMessage {
	message := unfurlLinks(event.Text)

	inputEvent := models.IncomingMessage{
		SubType:     event.SubType,
		Text:        message,
		ChannelID:   event.Channel,
		ChannelType: event.Type,
		UserID:      event.User,
		Timestamp:   event.Timestamp,
		ThreadID:    event.ThreadTimestamp,
	}

	// Skip messages sent by bots.
	if event.BotID != "" {
		inputEvent.UserID = ""
	}

	files := event.Files
	if len(files) > 0 {
		inputEvent.SnippetURL = files[0].URLPrivate
	}

	return inputEvent
}

// unfurlLinks unfurls Slack links to the original content.
func unfurlLinks(text string) string {
	if strings.Contains(text, "<http:") {
		text = linkRegexp.ReplaceAllString(text, `$1`)
	}

	if strings.Contains(text, "<mailto:") {
		text = emailRegexp.ReplaceAllString(text, `$1`)
	}

	return text
}

// parseEvent parses slack events.
//func (a *Assistant) parseEvent(rawEvent []byte) (slackevents.EventsAPIEvent, error) {
//	return slackevents.ParseEvent(rawEvent, slackevents.OptionNoVerifyToken())
//}

// verifyRequest verifies a request coming from Slack
//func (a *Assistant) verifyRequest(r *http.Request) error {
//	secretsVerifier, err := slack.NewSecretsVerifier(r.Header, a.credentialsCfg.SigningSecret)
//	if err != nil {
//		return errors.Wrap(err, "failed to init the secrets verifier")
//	}
//
//	body, err := ioutil.ReadAll(r.Body)
//	if err != nil {
//		return errors.Wrap(err, "failed to read the request body")
//	}
//
//	// Set a body with the same data we read.
//	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
//
//	if _, err := secretsVerifier.Write(body); err != nil {
//		return errors.Wrap(err, "failed to prepare the request body")
//	}
//
//	if err := secretsVerifier.Ensure(); err != nil {
//		return errors.Wrap(err, "failed to ensure a secret token")
//	}
//
//	return nil
//}
