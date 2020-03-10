package slack

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/nlopes/slack"
	"github.com/pkg/errors"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/structs"
	"gitlab.com/postgres-ai/joe/pkg/util"
)

const errorNotPublished = "Message not published yet"

// Bot reactions.
const (
	ReactionRunning = "hourglass_flowing_sand"
	ReactionError   = "x"
	ReactionOK      = "white_check_mark"
)

// statusMapping defines a status-reaction map.
var statusMapping = map[structs.MessageStatus]string{
	structs.StatusRunning: ReactionRunning,
	structs.StatusError:   ReactionError,
	structs.StatusOK:      ReactionOK,
}

// Subtypes of incoming messages.
const (
	subtypeGeneral   = ""
	subtypeFileShare = "file_share"
)

// supportedSubtypes defines supported message subtypes.
var supportedSubtypes = []string{
	subtypeGeneral,
	subtypeFileShare,
}

type Messenger struct {
	api    *slack.Client
	config *config.SlackConfig
}

func NewMessenger(api *slack.Client, cfg *config.SlackConfig) *Messenger {
	return &Messenger{
		api:    api,
		config: cfg,
	}
}

// ValidateIncomingMessage validates an incoming message.
func (m *Messenger) ValidateIncomingMessage(incomingMessage *structs.IncomingMessage) error {
	if incomingMessage == nil {
		return errors.New("input event must not be nil")
	}

	// Skip messages sent by bots.
	if incomingMessage.UserID == "" {
		return errors.New("userID must not be empty")
	}

	// Skip messages from threads.
	if incomingMessage.ThreadID != "" {
		return errors.New("skip message in thread")
	}

	if !util.Contains(supportedSubtypes, incomingMessage.SubType) {
		return errors.Errorf("subtype %q is not supported", incomingMessage.SubType)
	}

	if incomingMessage.ChannelID == "" {
		return errors.New("bad channelID specified")
	}

	return nil
}

// Message types of published messages.
const (
	messageTypeDefault   = ""
	messageTypeThread    = "thread"
	messageTypeEphemeral = "ephemeral"
)

func (m *Messenger) Publish(message *structs.Message) error {
	switch message.MessageType {
	case messageTypeDefault:
		_, timestamp, err := m.api.PostMessage(message.ChannelID, slack.MsgOptionText(message.Text, false))
		if err != nil {
			return errors.Wrap(err, "failed to post a message")
		}
		//message.ChannelID = channelID // Shouldn't change, but update just in case.
		message.MessageID = timestamp

	case messageTypeThread:
		_, _, err := m.api.PostMessage(message.ChannelID, slack.MsgOptionText(message.Text, false),
			slack.MsgOptionTS(message.ThreadID))
		if err != nil {
			return errors.Wrap(err, "failed to post a thread message")
		}

	case messageTypeEphemeral:
		timestamp, err := m.api.PostEphemeral(message.ChannelID, message.UserID, slack.MsgOptionText(message.Text, false))
		if err != nil {
			return errors.Wrap(err, "failed to post an ephemeral message")
		}

		message.MessageID = timestamp

	default:
		return errors.New("unknown message type")
	}

	return nil
}

func (m *Messenger) UpdateText(message *structs.Message) error {
	if !message.IsPublished() {
		return errors.New(errorNotPublished)
	}

	_, timestamp, _, err := m.api.UpdateMessage(message.ChannelID, message.MessageID, slack.MsgOptionText(message.Text, false))
	if err != nil {
		return errors.Wrap(err, "failed to update a message")
	}

	//message.ChannelID = channelId // Shouldn't change, but update just in case.
	message.MessageID = timestamp

	return nil
}

func (m *Messenger) UpdateStatus(message *structs.Message, status structs.MessageStatus) error {
	if !message.IsPublished() {
		return errors.New(errorNotPublished)
	}

	if status == message.Status {
		return nil
	}

	reaction, ok := statusMapping[status]
	if !ok {
		return errors.Errorf("unknown status given: %s", status)
	}

	msgRef := slack.NewRefToMessage(message.ChannelID, message.MessageID)

	// Add new reaction.
	if err := m.api.AddReaction(reaction, msgRef); err != nil {
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
		err = m.UpdateText(message)
	} else {
		message.SetText(errText)
		err = m.Publish(message)
	}

	if err != nil {
		return err
	}

	if err := m.UpdateStatus(message, structs.StatusError); err != nil {
		return errors.Wrap(err, "failed to update status")
	}

	if err := m.notifyAboutRequestFinish(message); err != nil {
		return errors.Wrap(err, "failed to notify about the request finish")
	}

	return nil
}

func (m *Messenger) OK(message *structs.Message) error {
	if err := m.UpdateStatus(message, structs.StatusOK); err != nil {
		return errors.Wrap(err, "failed to change reaction")
	}

	if err := m.notifyAboutRequestFinish(message); err != nil {
		return errors.Wrap(err, "failed to notify about finishing a long request")
	}

	return nil
}

func (m *Messenger) AddArtifact(title, explainResult, channelID, messageID string) (string, error) {
	filePlanWoExec, err := m.uploadFile(title, explainResult, channelID, messageID)
	if err != nil {
		log.Err("File upload failed:", err)
		return "", err
	}

	return filePlanWoExec.Permalink, nil
}

func (m *Messenger) uploadFile(title string, content string, channel string, ts string) (*slack.File, error) {
	const fileType = "txt"
	name := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	filename := fmt.Sprintf("%s.%s", name, fileType)

	params := slack.FileUploadParameters{
		Title:           title,
		Filetype:        "text",
		Filename:        filename,
		Content:         content,
		Channels:        []string{channel},
		ThreadTimestamp: ts,
	}

	file, err := m.api.UploadFile(params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to upload a file")
	}

	return file, nil
}

func (m *Messenger) DownloadArtifact(privateUrl string) ([]byte, error) {
	const (
		ContentTypeText     = "text/plain"
		HeaderAuthorization = "Authorization"
		HeaderContentType   = "Content-Type"
	)

	log.Dbg("Downloading snippet...")

	req, err := http.NewRequest(http.MethodGet, privateUrl, nil)
	if err != nil {
		return nil, errors.Wrap(err, "cannot initialize a download snippet request")
	}
	req.Header.Set(HeaderAuthorization, fmt.Sprintf("Bearer %s", m.config.AccessToken))

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "cannot download snippet")
	}
	defer resp.Body.Close()

	snippet, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "cannot read the snippet content")
	}

	// In case of bad authorization Slack sends HTML page with auth form.
	// Snippet should have a plain text content type.
	contentType := resp.Header.Get(HeaderContentType)
	if resp.StatusCode == http.StatusUnauthorized || !strings.Contains(contentType, ContentTypeText) {
		return nil, errors.Errorf("unauthorized to download snippet: response code %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("cannot download snippet: response code %d", resp.StatusCode)
	}

	log.Dbg("Snippet downloaded.")

	return snippet, nil
}

func (m *Messenger) notifyAboutRequestFinish(message *structs.Message) error {
	now := time.Now()
	if message.UserID == "" || now.Before(message.NotifyAt) {
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

	if err := m.Publish(threadMsg); err != nil {
		return errors.Wrap(err, "failed to publish a user mention")
	}

	return nil
}
