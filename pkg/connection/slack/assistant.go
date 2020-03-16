/*
2019 © Postgres.ai
*/

// Package slack provides the Slack implementation of the communication interface.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
	"github.com/pkg/errors"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/models"
	"gitlab.com/postgres-ai/joe/pkg/services/msgproc"
)

// Assistant provides a service for interaction with a communication channel.
type Assistant struct {
	credentialsCfg *config.Credentials
	msgProcessor   *msgproc.ProcessingService
	prefix         string
}

// SlackConfig defines a slack configuration parameters.
type SlackConfig struct {
	AccessToken   string
	SigningSecret string
}

// NewAssistant returns a new assistant service.
func NewAssistant(cfg *config.Credentials, msgProcessor *msgproc.ProcessingService) *Assistant {
	assistant := &Assistant{
		credentialsCfg: cfg,
		msgProcessor:   msgProcessor,
	}

	return assistant
}

// Register registers assistant handlers.
func (a *Assistant) Register() error {
	log.Dbg("URL-path prefix: ", a.prefix)

	for path, handleFunc := range a.handlers() {
		http.Handle(fmt.Sprintf("%s/%s", a.prefix, path), handleFunc)
	}

	return nil
}

// SetHandlerPrefix prepares and sets a handler URL-path prefix.
func (a *Assistant) SetHandlerPrefix(prefix string) {
	a.prefix = fmt.Sprintf("/%s", strings.Trim(prefix, "/"))
}

// CheckIdleSessions check the running user sessions for idleness.
func (a *Assistant) CheckIdleSessions(ctx context.Context) {
	a.msgProcessor.CheckIdleSessions(ctx)
}

func (a *Assistant) handlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"": a.handleEvent,
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

			msg := a.appMentionEventToIncomingMessage(ev)
			a.msgProcessor.ProcessAppMentionEvent(msg)

		case *slackevents.MessageEvent:
			log.Dbg("Event type: Message")

			if ev.BotID != "" {
				// Skip messages sent by bots.
				return
			}

			msg := a.messageEventToIncomingMessage(ev)
			a.msgProcessor.ProcessMessageEvent(msg)

		default:
			log.Dbg("Event filtered: Inner event type not supported")
		}

	default:
		log.Dbg("Event filtered: Event type not supported")
	}
}

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
func (a *Assistant) messageEventToIncomingMessage(event *slackevents.MessageEvent) models.IncomingMessage {
	inputEvent := models.IncomingMessage{
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
	secretsVerifier, err := slack.NewSecretsVerifier(r.Header, a.credentialsCfg.SigningSecret)
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
