package slack

import (
	"fmt"
	"time"

	"github.com/nlopes/slack"
	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/structs"
)

const errorNotPublished = "Message not published yet"

// Bot reactions.
const (
	ReactionRunning = "hourglass_flowing_sand"
	ReactionError   = "x"
	ReactionOK      = "white_check_mark"
)

type Messenger struct {
	api    *slack.Client
	config *config.SlackConfig
}

func New(api *slack.Client, cfg *config.SlackConfig) *Messenger {
	return &Messenger{
		api:    api,
		config: cfg,
	}
}

func (m *Messenger) Init() error {
	return nil
}

func (m *Messenger) Publish(message *structs.Message) error {
	channelID, timestamp, err := m.api.PostMessage(message.ChannelID, slack.MsgOptionText(message.Text, false))
	if err != nil {
		return err
	}

	message.ChannelID = channelID // Shouldn't change, but update just in case.
	message.MessageID = timestamp

	// TODO(akartasov): Support publishing to a thread.
	//_, _, err := m.Api.PostMessage(m.ChannelID,
	//	slack.MsgOptionText(text, false),
	//	slack.MsgOptionTS(threadTimestamp))

	// TODO(akartasov): Support ephemeral messages.
	//timestamp, err := m.Chat.Api.PostEphemeral(m.ChannelID, userId,
	//	slack.MsgOptionText(text, false))

	return nil
}

func (m *Messenger) Append(message *structs.Message) error {
	if !message.IsPublished() {
		return fmt.Errorf(errorNotPublished)
	}

	channelId, timestamp, _, err := m.api.UpdateMessage(message.ChannelID,
		message.MessageID, slack.MsgOptionText(message.Text, false))
	if err != nil {
		return err
	}

	message.ChannelID = channelId // Shouldn't change, but update just in case.
	message.MessageID = timestamp

	// TODO(akartasov): Support replace messages.
	//channelId, timestamp, _, err := m.Chat.Api.UpdateMessage(m.ChannelID,
	//	m.Timestamp, slack.MsgOptionText(text, false))
	//if err != nil {
	//	return err
	//}

	return nil
}

func (m *Messenger) UpdateStatus(message *structs.Message, status structs.MessageStatus) error {
	if !message.IsPublished() {
		return fmt.Errorf(errorNotPublished)
	}

	if status == message.Status {
		return nil
	}

	msgRef := slack.NewRefToMessage(message.ChannelID, message.MessageID)

	// Add new reaction.
	if err := m.api.AddReaction(string(status), msgRef); err != nil {
		message.SetStatus("")
		return err
	}

	// We have to add a new reaction before removing. In reverse order Slack UI will twitch.
	// TODO(anatoly): Remove reaction may fail, in that case we will lose data about added reaction.

	// Remove previous reaction.
	if message.Status != "" {
		if err := m.api.RemoveReaction(string(message.Status), msgRef); err != nil {
			return err
		}
	}

	message.Status = status

	return nil
}

func (m *Messenger) Fail(message *structs.Message, text string) error {
	var err error
	errText := fmt.Sprintf("ERROR: %s", text)

	if message.IsPublished() {
		message.AppendText(errText)
		err = m.Append(message)
	} else {
		message.SetText(errText)
		err = m.Publish(message)
	}

	if err != nil {
		return err
	}

	if err := m.UpdateStatus(message, ReactionError); err != nil {
		return errors.Wrap(err, "failed to update status")
	}

	if err := m.notifyAboutRequestFinish(message); err != nil {
		return errors.Wrap(err, "failed to notify about the request finish")
	}

	return nil
}

func (m *Messenger) OK(message *structs.Message) error {
	if err := m.UpdateStatus(message, ReactionOK); err != nil {
		return errors.Wrap(err, "failed to change reaction")
	}

	if err := m.notifyAboutRequestFinish(message); err != nil {
		return errors.Wrap(err, "failed to notify about finishing a long request")
	}

	return nil
}

func (m *Messenger) notifyAboutRequestFinish(message *structs.Message) error {
	now := time.Now()
	if message.UserID == "" || now.Before(message.CreatedAt) {
		return nil
	}

	text := fmt.Sprintf("<@%s> :point_up_2:", message.UserID)

	if err := m.publishToThread(message, text); err != nil {
		return errors.Wrap(err, "failed to publish a user mention")
	}

	return nil
}

func (m *Messenger) publishToThread(message *structs.Message, text string) error {
	threadMsg := &structs.Message{
		ChannelID: message.ChannelID,
		ThreadID:  message.MessageID,
		UserID:    message.UserID,
		Text:      text,
	}

	//_, _, err := message.Api.PostMessage(message.ChannelID,
	//	slack.MsgOptionText(text, false),
	//	slack.MsgOptionTS(threadTimestamp))

	if err := m.Publish(threadMsg); err != nil {
		return errors.Wrap(err, "failed to publish a user mention")
	}

	return nil
}
