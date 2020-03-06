package msgproc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/nlopes/slack/slackevents"
	"github.com/pkg/errors"
	"gitlab.com/postgres-ai/database-lab/pkg/client/dblabapi"
	"gitlab.com/postgres-ai/database-lab/pkg/log"
	"gitlab.com/postgres-ai/database-lab/pkg/util"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/bot/command"
	"gitlab.com/postgres-ai/joe/pkg/ee"
	"gitlab.com/postgres-ai/joe/pkg/services/messenger"
	"gitlab.com/postgres-ai/joe/pkg/services/usermanager"
	"gitlab.com/postgres-ai/joe/pkg/structs"
	"gitlab.com/postgres-ai/joe/pkg/transmission/pgtransmission"
	"gitlab.com/postgres-ai/joe/pkg/util/text"
)

const COMMAND_EXPLAIN = "explain"
const COMMAND_EXEC = "exec"
const COMMAND_RESET = "reset"
const COMMAND_HELP = "help"
const COMMAND_HYPO = "hypo"
const COMMAND_PLAN = "plan"

const COMMAND_PSQL_D = `\d`
const COMMAND_PSQL_DP = `\d+`
const COMMAND_PSQL_DT = `\dt`
const COMMAND_PSQL_DTP = `\dt+`
const COMMAND_PSQL_DI = `\di`
const COMMAND_PSQL_DIP = `\di+`
const COMMAND_PSQL_L = `\l`
const COMMAND_PSQL_LP = `\l+`
const COMMAND_PSQL_DV = `\dv`
const COMMAND_PSQL_DVP = `\dv+`
const COMMAND_PSQL_DM = `\dm`
const COMMAND_PSQL_DMP = `\dm+`

var supportedCommands = []string{
	COMMAND_EXPLAIN,
	COMMAND_PLAN,
	COMMAND_HYPO,
	COMMAND_EXEC,
	COMMAND_RESET,
	COMMAND_HELP,

	COMMAND_PSQL_D,
	COMMAND_PSQL_DP,
	COMMAND_PSQL_DT,
	COMMAND_PSQL_DTP,
	COMMAND_PSQL_DI,
	COMMAND_PSQL_DIP,
	COMMAND_PSQL_L,
	COMMAND_PSQL_LP,
	COMMAND_PSQL_DV,
	COMMAND_PSQL_DVP,
	COMMAND_PSQL_DM,
	COMMAND_PSQL_DMP,
}

var allowedPsqlCommands = []string{
	COMMAND_PSQL_D,
	COMMAND_PSQL_DP,
	COMMAND_PSQL_DT,
	COMMAND_PSQL_DTP,
	COMMAND_PSQL_DI,
	COMMAND_PSQL_DIP,
	COMMAND_PSQL_L,
	COMMAND_PSQL_LP,
	COMMAND_PSQL_DV,
	COMMAND_PSQL_DVP,
	COMMAND_PSQL_DM,
	COMMAND_PSQL_DMP,
}

type ProcessingService struct {
	//msgValidator    MessageEventValidator
	Messenger messenger.Messenger
	DBLab     *dblabapi.Client
	//PlatformManager
	usermanager.UserManager
	//Auditor
	//Limiter
}

const SUBTYPE_GENERAL = ""
const SUBTYPE_FILE_SHARE = "file_share"

var supportedSubtypes = []string{
	SUBTYPE_GENERAL,
	SUBTYPE_FILE_SHARE,
}

var spaceRegex = regexp.MustCompile(`\s+`)

func (s *ProcessingService) ProcessMessageEvent(ev *slackevents.MessageEvent) {
	// TODO(akartasov): Implement.
	var err error

	// Filter event.

	// Skip messages sent by bots.
	if ev.User == "" || ev.BotID != "" {
		log.Dbg("Message filtered: Bot")
		return
	}

	// Skip messages from threads.
	if ev.ThreadTimeStamp != "" {
		log.Dbg("Message filtered: Message in thread")
		return
	}

	if !util.Contains(supportedSubtypes, ev.SubType) {
		log.Dbg("Message filtered: Subtype not supported")
		return
	}

	ch := ev.Channel

	if ch == "" {
		log.Err("Bad channelID specified")
		return
	}

	// Get user or create a new one.
	user, err := s.UserManager.CreateUser(ev.User)
	if err != nil {
		log.Err(errors.Wrap(err, "failed to get user"))

		if err := s.Messenger.Fail(structs.NewMessage(ch), err.Error()); err != nil {
			log.Err(errors.Wrap(err, "failed to get user"))
			return
		}

		return
	}

	user.Session.LastActionTs = time.Now()
	if !util.Contains(user.Session.ChannelIDs, ch) {
		user.Session.ChannelIDs = append(user.Session.ChannelIDs, ch)
	}

	// Filter and prepare message.
	message := strings.TrimSpace(ev.Text)
	message = strings.TrimLeft(message, "`")
	message = strings.TrimRight(message, "`")
	message = formatMessage(message)

	// TODO (akartasov): Download snippet to message.
	// Get command from snippet if exists. Snippets allow longer queries support.
	//files := ev.Files
	//if len(files) > 0 {
	//	log.Dbg("Using attached file as message")
	//	file := files[0]
	//	snippet, err := s.Chat.DownloadSnippet(file.URLPrivate)
	//	if err != nil {
	//		log.Err(err)
	//
	//		msg, _ := s.Chat.NewMessage(ch)
	//		msg.Publish(" ")
	//		msg.Fail(err.Error())
	//		return
	//	}
	//
	//	message = string(snippet)
	//}

	if len(message) == 0 {
		log.Dbg("Message filtered: Empty")
		return
	}

	// Replace any number of spaces, tab, new lines with single space.
	message = spaceRegex.ReplaceAllString(message, " ")

	// Message: "command query(optional)".
	parts := strings.SplitN(message, " ", 2)
	receivedCommand := strings.ToLower(parts[0])

	query := ""
	if len(parts) > 1 {
		query = parts[1]
	}

	s.showBotHints(ev, receivedCommand, query)

	if !util.Contains(supportedCommands, receivedCommand) {
		log.Dbg("Message filtered: Not a command")
		return
	}

	if err := user.RequestQuota(); err != nil {
		log.Err("Quota: ", err)

		if err := s.Messenger.Fail(structs.NewMessage(ch), err.Error()); err != nil {
			log.Err(errors.Wrap(err, "failed to request quotas"))
			return
		}

		return
	}

	// We want to save message height space for more valuable info.
	queryPreview := strings.ReplaceAll(query, "\n", " ")
	queryPreview = strings.ReplaceAll(queryPreview, "\t", " ")
	queryPreview, _ = text.CutText(queryPreview, QUERY_PREVIEW_SIZE, SEPARATOR_ELLIPSIS)

	if s.Config.AuditEnabled {
		audit, err := json.Marshal(ee.Audit{
			Id:       user.UserInfo.ID,
			Name:     user.UserInfo.Name,
			RealName: user.UserInfo.RealName,
			Command:  receivedCommand,
			Query:    query,
		})

		if err != nil {
			if err := s.Messenger.Fail(structs.NewMessage(ch), err.Error()); err != nil {
				log.Err(errors.Wrap(err, "failed to marshal Audit struct"))
				return
			}

			return
		}

		log.Audit(string(audit))
	}

	msgText := fmt.Sprintf("```%s %s```\n", receivedCommand, queryPreview)

	// Show `help` command without initializing of a session.
	if receivedCommand == COMMAND_HELP {
		hMsg := structs.NewMessage(ch)

		msgText = appendHelp(msgText, s.Config.Version)
		msgText = appendSessionId(msgText, user)
		hMsg.SetText(msgText)

		if err := s.Messenger.Publish(hMsg); err != nil {
			// TODO(anatoly): Retry.
			log.Err("Bot: Cannot publish a message", err)
		}

		return
	}

	if err := s.runSession(context.TODO(), user, ch); err != nil {
		log.Err(err)
		return
	}

	msg := structs.NewMessage(ch)

	msgText = appendSessionId(msgText, user)
	msg.SetText(msgText)

	if err := s.Messenger.Publish(msg); err != nil {
		// TODO(anatoly): Retry.
		log.Err("Bot: Cannot publish a message", err)
		return
	}

	remindDuration := time.Duration(s.Config.MinNotifyDurationMinutes) * time.Minute
	if err := msg.SetLongRunningTimestamp(remindDuration); err != nil {
		log.Err(err)
	}
	msg.SetChatUserID(user.UserInfo.ID)

	s.Messenger.UpdateStatus(msg, structs.StatusRunning)

	apiCmd := &api.ApiCommand{
		AccessToken: s.Config.ApiToken,
		ApiURL:      s.Config.ApiUrl,
		SessionId:   user.Session.PlatformSessionId,
		Command:     receivedCommand,
		Query:       query,
		SlackTs:     ev.TimeStamp,
	}

	const maxRetryCounter = 1

	// TODO(akartasov): Refactor commands and create retrier.
	for iteration := 0; iteration <= maxRetryCounter; iteration++ {
		switch {
		case receivedCommand == COMMAND_EXPLAIN:
			err = command.Explain(s.Messenger, apiCmd, msg, s.Config, user.Session.CloneConnection)

		case receivedCommand == COMMAND_PLAN:
			planCmd := command.NewPlan(apiCmd, msg, user.Session.CloneConnection, s.Messenger)
			err = planCmd.Execute()

		case receivedCommand == COMMAND_EXEC:
			execCmd := command.NewExec(apiCmd, msg, user.Session.CloneConnection, s.Messenger)
			err = execCmd.Execute()

		case receivedCommand == COMMAND_RESET:
			err = command.ResetSession(context.TODO(), apiCmd, msg, s.DBLab, user.Session.Clone.ID, s.Messenger)

		case receivedCommand == COMMAND_HYPO:
			hypoCmd := command.NewHypo(apiCmd, msg, user.Session.CloneConnection, s.Messenger)
			err = hypoCmd.Execute()

		case util.Contains(allowedPsqlCommands, receivedCommand):
			runner := pgtransmission.NewPgTransmitter(user.Session.ConnParams, pgtransmission.LogsEnabledDefault)
			err = command.Transmit(apiCmd, msg, s.Messenger, runner)
		}

		if err != nil {
			if _, ok := err.(*net.OpError); !ok || iteration == maxRetryCounter {

				s.Messenger.Fail(msg, err.Error())
				apiCmd.Fail(err.Error())
				return
			}

			if s.isActiveSession(context.TODO(), user.Session.Clone.ID) {
				continue
			}

			msg.AppendText("Session was closed by Database Lab.\n")
			if err := s.Messenger.Append(msg); err != nil {
				log.Err(fmt.Sprintf("failed to append message on session close: %+v", err))
			}
			s.stopSession(user)

			if err := s.runSession(context.TODO(), user, msg.ChannelID); err != nil {
				log.Err(err)
				return
			}
		}

		break
	}

	if s.Config.HistoryEnabled {
		_, err := apiCmd.Post()
		if err != nil {
			log.Err(err)

			s.Messenger.Fail(msg, err.Error())
			return
		}
	}

	if err := s.Messenger.OK(msg); err != nil {
		log.Err(err)
	}

}

// Show bot usage hints.
func (s *ProcessingService) showBotHints(ev *slackevents.MessageEvent, command string, query string) {
	parts := strings.SplitN(query, " ", 2)
	firstQueryWord := strings.ToLower(parts[0])

	checkQuery := len(firstQueryWord) > 0 && command == COMMAND_EXEC

	if (checkQuery && util.Contains(hintExplainDmlWords, firstQueryWord)) ||
		util.Contains(hintExplainDmlWords, command) {
		msg := structs.NewMessage(ev.Channel)
		msg.MessageType = "ephemeral"
		msg.UserID = ev.User
		msg.SetText(HINT_EXPLAIN)

		if err := s.Messenger.Publish(msg); err != nil {
			log.Err("Hint explain:", err)
		}
	}

	if util.Contains(hintExecDdlWords, command) {
		msg := structs.NewMessage(ev.Channel)
		msg.MessageType = "ephemeral"
		msg.UserID = ev.User
		msg.SetText(HINT_EXEC)

		if err := s.Messenger.Publish(msg); err != nil {
			log.Err("Hint exec:", err)
		}
	}
}

// TODO(akartasov): refactor to slice of bytes.
func formatMessage(msg string) string {
	// Slack escapes some characters
	// https://api.slack.com/docs/message-formatting#how_to_escape_characters
	msg = strings.ReplaceAll(msg, "&amp;", "&")
	msg = strings.ReplaceAll(msg, "&lt;", "<")
	msg = strings.ReplaceAll(msg, "&gt;", ">")

	// Smart quotes could be substituted automatically on macOS.
	// Replace smart quotes (“...”) with straight quotes ("...").
	msg = strings.ReplaceAll(msg, "“", "\"")
	msg = strings.ReplaceAll(msg, "”", "\"")
	msg = strings.ReplaceAll(msg, "‘", "'")
	msg = strings.ReplaceAll(msg, "’", "'")

	return msg
}

func appendSessionId(text string, u *usermanager.User) string {
	s := "No session\n"

	if u != nil && u.Session.Clone != nil && u.Session.Clone.ID != "" {
		sessionId := u.Session.Clone.ID

		// Use session ID from platform if it's defined.
		if u.Session.PlatformSessionId != "" {
			sessionId = u.Session.PlatformSessionId
		}

		s = fmt.Sprintf("Session: `%s`\n", sessionId)
	}

	return text + s
}

func appendHelp(text string, version string) string {
	return text + MSG_HELP + fmt.Sprintf("Version: %s\n", version)
}
