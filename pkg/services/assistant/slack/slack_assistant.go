package slack

import (
	"bytes"
	"encoding/json"
	"html"
	"io/ioutil"
	"net/http"

	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
	"github.com/pkg/errors"
	"gitlab.com/postgres-ai/database-lab/pkg/client/dblabapi"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/services/msgproc"
	"gitlab.com/postgres-ai/joe/pkg/services/usermanager"
	"gitlab.com/postgres-ai/joe/pkg/structs"
)

type Assistant struct {
	ServiceConfig *config.SlackConfig
	msgProcessor  msgproc.ProcessingService
}

func NewAssistant(cfg *config.SlackConfig, botCfg config.Bot, slackMsg *Messenger, dblab *dblabapi.Client) *Assistant {
	assistant := &Assistant{
		ServiceConfig: cfg,
		msgProcessor: msgproc.ProcessingService{
			Messenger:   slackMsg,
			DBLab:       dblab,
			UserManager: usermanager.NewUserManager(slackMsg, botCfg),
		},
	}

	return assistant
}

func (a *Assistant) Init() error {
	for path, handleFunc := range a.Handlers() {
		http.Handle(path, handleFunc)
	}

	return nil
}

func (a *Assistant) Handlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/": a.handleEvent,
	}
}

func (a *Assistant) handleEvent(w http.ResponseWriter, r *http.Request) {
	log.Msg("Request received:", html.EscapeString(r.URL.Path))

	// TODO(anatoly): Respond time according to Slack API timeouts policy.
	// Slack sends retries in case of timedout responses.
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
			a.processAppMentionEvent(ev)

		case *slackevents.MessageEvent:
			log.Dbg("Event type: Message")

			if ev.BotID != "" {
				// Skip messages sent by bots.
				return
			}

			msg := a.slackEventToIncomingMessage(ev)
			a.msgProcessor.ProcessMessageEvent(msg)

		default:
			log.Dbg("Event filtered: Inner event type not supported")
		}

	default:
		log.Dbg("Event filtered: Event type not supported")
	}
}

// slackEventToIncomingMessage converts a Slack message event to the standard incoming message.
func (a *Assistant) slackEventToIncomingMessage(event *slackevents.MessageEvent) structs.IncomingMessage {
	inputEvent := structs.IncomingMessage{
		SubType:     event.SubType,
		Text:        event.Text,
		ChannelID:   event.Channel,
		ChannelType: event.ChannelType,
		UserID:      event.User,
		Timestamp:   event.TimeStamp,
		ThreadID:    event.ThreadTimeStamp,
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

// parseEvent parses slack events.
func (a *Assistant) parseEvent(rawEvent []byte) (slackevents.EventsAPIEvent, error) {
	return slackevents.ParseEvent(rawEvent, slackevents.OptionNoVerifyToken())
}

// verifyRequest verifies a request coming from Slack
func (a *Assistant) verifyRequest(r *http.Request) error {
	secretsVerifier, err := slack.NewSecretsVerifier(r.Header, a.ServiceConfig.SigningSecret)
	if err != nil {
		return errors.Wrap(err, "failed to init the secrets verifier")
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read the request body")
	}

	// Set a body with the same data we read.
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	if _, err := secretsVerifier.Write(body); err != nil {
		return errors.Wrap(err, "failed to prepare the request body")
	}

	if err := secretsVerifier.Ensure(); err != nil {
		return errors.Wrap(err, "failed to ensure a secret token")
	}

	return nil
}

func (a *Assistant) processAppMentionEvent(ev *slackevents.AppMentionEvent) {
	msg := structs.NewMessage(ev.Channel)

	msg.SetText("What's up? Send `help` to see the list of available commands.")

	if err := a.msgProcessor.Messenger.Publish(msg); err != nil {
		// TODO(anatoly): Retry.
		log.Err("Bot: Cannot publish a message", err)
		return
	}
}
