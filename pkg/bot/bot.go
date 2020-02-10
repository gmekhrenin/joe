/*
2019 © Postgres.ai
*/

package bot

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize/english"
	"github.com/hako/durafmt"
	_ "github.com/lib/pq"
	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/sethvargo/go-password/password"

	"gitlab.com/postgres-ai/database-lab/pkg/client/dblabapi"
	"gitlab.com/postgres-ai/database-lab/pkg/log"
	"gitlab.com/postgres-ai/database-lab/pkg/models"

	"gitlab.com/postgres-ai/joe/pkg/bot/api"
	"gitlab.com/postgres-ai/joe/pkg/bot/command"
	"gitlab.com/postgres-ai/joe/pkg/chatapi"
	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/dblab"
	"gitlab.com/postgres-ai/joe/pkg/pgexplain"
	"gitlab.com/postgres-ai/joe/pkg/transmission/pgtransmission"
	"gitlab.com/postgres-ai/joe/pkg/util"
	"gitlab.com/postgres-ai/joe/pkg/util/text"
)

const COMMAND_EXPLAIN = "explain"
const COMMAND_EXEC = "exec"
const COMMAND_RESET = "reset"
const COMMAND_HELP = "help"

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

const SUBTYPE_GENERAL = ""
const SUBTYPE_FILE_SHARE = "file_share"

var supportedSubtypes = []string{
	SUBTYPE_GENERAL,
	SUBTYPE_FILE_SHARE,
}

const QUERY_PREVIEW_SIZE = 400

const InactiveCloneCheckInterval = time.Minute

const MSG_HELP = "• `explain` — analyze your query (SELECT, INSERT, DELETE, UPDATE or WITH) and generate recommendations\n" +
	"• `exec` — execute any query (for example, CREATE INDEX)\n" +
	"• `reset` — revert the database to the initial state (usually takes less than a minute, :warning: all changes will be lost)\n" +
	"• `\\d`, `\\d+`, `\\dt`, `\\dt+`, `\\di`, `\\di+`, `\\l`, `\\l+`, `\\dv`, `\\dv+`, `\\dm`, `\\dm+` — psql meta information commands\n" +
	"• `help` — this message\n"

const MsgSessionStarting = "Starting new session...\n\n"

const MsgSessionForewordTpl = "• Say 'help' to see the full list of commands.\n" +
	"• Sessions are fully independent. Feel free to do anything.\n" +
	"• The session will be destroyed after %s of inactivity.\n" +
	"• EXPLAIN plans here are expected to be identical to production plans.\n" +
	"• The actual timing values may differ from production because actual caches in DB Lab are smaller. " +
	"However, the number of bytes and pages/buffers in plans are identical to production.\n" +
	"\nMade with :hearts: by Postgres.ai. Bug reports, ideas, and merge requests are welcome: https://gitlab.com/postgres-ai/joe \n" +
	"\nJoe version: %s.\nSnapshot data state at: %s."

const RCTN_RUNNING = "hourglass_flowing_sand"
const RCTN_OK = "white_check_mark"

const SEPARATOR_ELLIPSIS = "\n[...SKIP...]\n"

const HINT_EXPLAIN = "Consider using `explain` command for DML statements. See `help` for details."
const HINT_EXEC = "Consider using `exec` command for DDL statements. See `help` for details."

const joeUserNamePrefix = "joe_"

// Constants for autogenerated passwords.
const (
	PasswordLength     = 16
	PasswordMinDigits  = 4
	PasswordMinSymbols = 0
)

var hintExplainDmlWords = []string{"insert", "select", "update", "delete", "with"}
var hintExecDdlWords = []string{"alter", "create", "drop", "set"}

var spaceRegex = regexp.MustCompile(`\s+`)

type Config struct {
	ConnStr       string
	Port          uint
	Explain       pgexplain.ExplainConfig
	QuotaLimit    uint
	QuotaInterval uint // Seconds.
	IdleInterval  uint // Seconds.

	DBLab DBLabInstance

	ApiUrl         string
	ApiToken       string
	ApiProject     string
	HistoryEnabled bool

	Version string
}

// DBLabInstance contains Database Lab config.
type DBLabInstance struct {
	URL     string
	Token   string
	DBName  string // TODO(akartasov): Make a dynamically used name.
	SSLMode string
}

type Bot struct {
	Config     config.Bot
	Chat       *chatapi.Chat
	DBLab      *dblabapi.Client
	usersMutex sync.RWMutex
	Users      map[string]*User // Slack UID -> User.
}

type Audit struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	RealName string `json:"realName"`
	Command  string `json:"command"`
	Query    string `json:"query"`
}

type User struct {
	ChatUser *slack.User
	Session  UserSession
}

type UserSession struct {
	PlatformSessionId string

	QuotaTs       time.Time
	QuotaCount    uint
	QuotaLimit    uint
	QuotaInterval uint

	LastActionTs time.Time
	IdleInterval uint

	ChannelIDs []string

	Clone           *models.Clone
	ConnParams      dblab.Clone
	CloneConnection *sql.DB
}

func NewBot(cfg config.Bot, chat *chatapi.Chat, dbLab *dblabapi.Client) *Bot {
	bot := Bot{
		Config: cfg,
		Chat:   chat,
		DBLab:  dbLab,
		Users:  make(map[string]*User),
	}
	return &bot
}

func NewUser(chatUser *slack.User, cfg config.Bot) *User {
	user := User{
		ChatUser: chatUser,
		Session: UserSession{
			QuotaTs:       time.Now(),
			QuotaCount:    0,
			QuotaLimit:    cfg.QuotaLimit,
			QuotaInterval: cfg.QuotaInterval,
			LastActionTs:  time.Now(),
		},
	}

	return &user
}

func (b *Bot) checkIdleSessions(ctx context.Context) error {
	channelsToNotify := make(map[string][]string)

	// TODO(akartasov): Fix data races.
	for _, user := range b.Users {
		if user == nil || user.Session.Clone == nil {
			continue
		}

		minutesAgoSinceLastAction := util.MinutesAgo(user.Session.LastActionTs)

		if minutesAgoSinceLastAction < user.Session.Clone.Metadata.MaxIdleMinutes {
			continue
		}

		if b.isActiveSession(ctx, user.Session.Clone.ID) {
			continue
		}

		log.Dbg("Session idle: %v %v", user, user.Session)

		b.stopSession(user)

		for _, channelID := range user.Session.ChannelIDs {
			chatUserID := user.ChatUser.ID

			channelsToNotify[channelID] = append(channelsToNotify[channelID], chatUserID)
		}
	}

	// Publish message in every channel with a list of users.
	for channelID, chatUserIDs := range channelsToNotify {
		if len(chatUserIDs) == 0 {
			continue
		}

		formattedUserList := make([]string, 0, len(chatUserIDs))
		for _, chatUserID := range chatUserIDs {
			formattedUserList = append(formattedUserList, fmt.Sprintf("<@%s>", chatUserID))
		}

		msgText := "Stopped idle sessions for: " + strings.Join(formattedUserList, ", ")

		msg, _ := b.Chat.NewMessage(channelID)
		err := msg.Publish(msgText)
		if err != nil {
			log.Err("Bot: Cannot publish a message", err)
		}
	}

	return nil
}

func (b *Bot) RunServer() {
	// Check idle sessions.
	_ = util.RunInterval(InactiveCloneCheckInterval, func() {
		log.Dbg("Check idle sessions")
		b.checkIdleSessions(context.TODO())
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b.handleEvent(w, r)
	})

	port := b.Config.Port
	log.Msg(fmt.Sprintf("Server start listening on localhost:%d", port))
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	log.Err("HTTP server error:", err)
}

func (b *Bot) handleEvent(w http.ResponseWriter, r *http.Request) {
	log.Msg("Request received:", html.EscapeString(r.URL.Path))

	// TODO(anatoly): Respond time according to Slack API timeouts policy.
	// Slack sends retries in case of timedout responses.
	if r.Header.Get("X-Slack-Retry-Num") != "" {
		log.Dbg("Message filtered: Slack Retry")
		return
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(r.Body)
	body := buf.String()

	eventsAPIEvent, err := b.Chat.ParseEvent(body)
	if err != nil {
		log.Err("Event parse error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch eventsAPIEvent.Type {
	// Used to verified bot's API URL for Slack.
	case slackevents.URLVerification:
		log.Dbg("Event type: URL verification")
		var r *slackevents.ChallengeResponse

		err := json.Unmarshal([]byte(body), &r)
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
			b.processAppMentionEvent(ev)

		case *slackevents.MessageEvent:
			log.Dbg("Event type: Message")
			b.processMessageEvent(ev)

		default:
			log.Dbg("Event filtered: Inner event type not supported")
		}

	default:
		log.Dbg("Event filtered: Event type not supported")
	}
}

func (b *Bot) processAppMentionEvent(ev *slackevents.AppMentionEvent) {
	var err error

	msg, _ := b.Chat.NewMessage(ev.Channel)
	err = msg.Publish("What's up? Send `help` to see the list of available commands.")
	if err != nil {
		// TODO(anatoly): Retry.
		log.Err("Bot: Cannot publish a message", err)
		return
	}
}

func (b *Bot) processMessageEvent(ev *slackevents.MessageEvent) {
	var err error

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
	message := strings.TrimSpace(ev.Text)
	message = strings.TrimLeft(message, "`")
	message = strings.TrimRight(message, "`")

	// Get user or create a new one.
	user, err := b.getUser(ev.User)
	if err != nil {
		log.Err(errors.Wrap(err, "failed to get user"))

		msg, _ := b.Chat.NewMessage(ch)
		msg.Publish(" ")
		msg.Fail(err.Error())

		return
	}

	user.Session.LastActionTs = time.Now()
	if !util.Contains(user.Session.ChannelIDs, ch) {
		user.Session.ChannelIDs = append(user.Session.ChannelIDs, ch)
	}

	message = formatSlackMessage(message)

	// Get command from snippet if exists. Snippets allow longer queries support.
	files := ev.Files
	if len(files) > 0 {
		log.Dbg("Using attached file as message")
		file := files[0]
		snippet, err := b.Chat.DownloadSnippet(file.URLPrivate)
		if err != nil {
			log.Err(err)

			msg, _ := b.Chat.NewMessage(ch)
			msg.Publish(" ")
			msg.Fail(err.Error())
			return
		}

		message = string(snippet)
	}

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

	b.showBotHints(ev, receivedCommand, query)

	if !util.Contains(supportedCommands, receivedCommand) {
		log.Dbg("Message filtered: Not a command")
		return
	}

	err = user.requestQuota()
	if err != nil {
		log.Err("Quota: ", err)
		msg, _ := b.Chat.NewMessage(ch)
		msg.Publish(" ")
		msg.Fail(err.Error())
		return
	}

	// We want to save message height space for more valuable info.
	queryPreview := strings.ReplaceAll(query, "\n", " ")
	queryPreview = strings.ReplaceAll(queryPreview, "\t", " ")
	queryPreview, _ = text.CutText(queryPreview, QUERY_PREVIEW_SIZE, SEPARATOR_ELLIPSIS)

	audit, err := json.Marshal(Audit{
		Id:       user.ChatUser.ID,
		Name:     user.ChatUser.Name,
		RealName: user.ChatUser.RealName,
		Command:  receivedCommand,
		Query:    query,
	})
	if err != nil {
		msg, _ := b.Chat.NewMessage(ch)
		msg.Publish(" ")
		msg.Fail(err.Error())
		return
	}
	log.Audit(string(audit))

	msgText := fmt.Sprintf("```%s %s```\n", receivedCommand, queryPreview)

	// Show `help` command without initializing of a session.
	if receivedCommand == COMMAND_HELP {
		msgText = appendHelp(msgText, b.Config.Version)
		msgText = appendSessionId(msgText, user)

		hMsg, _ := b.Chat.NewMessage(ch)
		err = hMsg.Publish(msgText)
		if err != nil {
			// TODO(anatoly): Retry.
			log.Err("Bot: Cannot publish a message", err)
		}

		return
	}

	if err := b.runSession(context.TODO(), user, ch); err != nil {
		log.Err(err)
		return
	}

	msgText = appendSessionId(msgText, user)

	msg, err := b.Chat.NewMessage(ch)
	if err != nil {
		log.Err("Bot: Cannot create a message", err)
		return
	}

	if err := msg.Publish(msgText); err != nil {
		// TODO(anatoly): Retry.
		log.Err("Bot: Cannot publish a message", err)
		return
	}

	runMsg(msg)

	apiCmd := &api.ApiCommand{
		AccessToken: b.Config.ApiToken,
		ApiURL:      b.Config.ApiUrl,
		SessionId:   user.Session.PlatformSessionId,
		Command:     receivedCommand,
		Query:       query,
		SlackTs:     ev.TimeStamp,
		Error:       "",
	}

	const maxRetryCounter = 1

	// TODO(akartasov): Refactor commands and create retrier.
	for iteration := 0; iteration <= maxRetryCounter; iteration++ {
		switch {
		case receivedCommand == COMMAND_EXPLAIN:
			err = command.Explain(b.Chat, apiCmd, msg, b.Config, user.Session.CloneConnection)

		case receivedCommand == COMMAND_EXEC:
			err = command.Exec(apiCmd, msg, user.Session.CloneConnection)

		case receivedCommand == COMMAND_RESET:
			err = command.ResetSession(context.TODO(), apiCmd, msg, b.DBLab, user.Session.Clone.ID)

		case util.Contains(allowedPsqlCommands, receivedCommand):
			runner := pgtransmission.NewPgTransmitter(user.Session.ConnParams, pgtransmission.LogsEnabledDefault)
			err = command.Transmit(apiCmd, msg, b.Chat, runner)
		}

		if err != nil {
			if _, ok := err.(*net.OpError); !ok || iteration == maxRetryCounter {
				msg.Fail(err.Error())
				apiCmd.Fail(err.Error())
				return
			}

			if b.isActiveSession(context.TODO(), user.Session.Clone.ID) {
				continue
			}

			if err := msg.Append("Session was closed by Database Lab.\n"); err != nil {
				log.Err(fmt.Sprintf("failed to append message on session close: %+v", err))
			}
			b.stopSession(user)

			if err := b.runSession(context.TODO(), user, msg.ChannelID); err != nil {
				log.Err(err)
				return
			}
		}

		break
	}

	if b.Config.HistoryEnabled {
		_, err := apiCmd.Post()
		if err != nil {
			log.Err(err)
			msg.Fail(err.Error())
			return
		}
	}

	okMsg(msg)
}

// runSession starts a user session if not exists.
func (b *Bot) runSession(ctx context.Context, user *User, channelID string) error {
	sMsg, _ := b.Chat.NewMessage(channelID)

	messageText := strings.Builder{}

	if user.Session.Clone != nil {
		return nil
	}

	// Stop clone session if not active.
	b.stopSession(user)

	messageText.WriteString(MsgSessionStarting)
	sMsg.Publish(messageText.String())
	messageText.Reset()

	runMsg(sMsg)

	clone, err := b.createDBLabClone(ctx, user)
	if err != nil {
		sMsg.Fail(err.Error())
		return err
	}

	if err := sMsg.Append(getForeword(time.Duration(clone.Metadata.MaxIdleMinutes)*time.Minute,
		b.Config.Version, clone.Snapshot.DataStateAt)); err != nil {
		sMsg.Fail(err.Error())
		return errors.Wrap(err, "failed to append message with a foreword")
	}

	if clone.DB == nil {
		return errors.New("failed to get connection params")
	}

	dblabClone := b.buildDBLabCloneConn(clone.DB)

	db, err := initConn(dblabClone)
	if err != nil {
		return errors.Wrap(err, "failed to init database connection")
	}

	user.Session.ConnParams = dblabClone
	user.Session.Clone = clone
	user.Session.CloneConnection = db

	if b.Config.HistoryEnabled {
		if err := b.createPlatformSession(user, sMsg.ChannelID); err != nil {
			sMsg.Fail(err.Error())
			return err
		}
	}

	sessionID := user.Session.Clone.ID
	if user.Session.PlatformSessionId != "" {
		sessionID = user.Session.PlatformSessionId
	}

	if err := sMsg.Append(fmt.Sprintf("Session started: `%s`", sessionID)); err != nil {
		sMsg.Fail(err.Error())
		return errors.Wrap(err, "failed to append message about session start")
	}

	okMsg(sMsg)

	return nil
}

func (b *Bot) buildDBLabCloneConn(DBParams *models.Database) dblab.Clone {
	return dblab.Clone{
		Name:     b.Config.DBLab.DBName,
		Host:     DBParams.Host,
		Port:     DBParams.Port,
		Username: DBParams.Username,
		Password: DBParams.Password,
		SSLMode:  b.Config.DBLab.SSLMode,
	}
}

func initConn(dblabClone dblab.Clone) (*sql.DB, error) {
	db, err := sql.Open("postgres", dblabClone.ConnectionString())
	if err != nil {
		log.Err("DB connection:", err)
		return nil, err
	}

	if err := db.PingContext(context.TODO()); err != nil {
		return nil, errors.WithStack(err)
	}

	return db, nil
}

// isActiveSession checks if current user session is active.
func (b *Bot) isActiveSession(ctx context.Context, cloneID string) bool {
	clone, err := b.DBLab.GetClone(ctx, cloneID)
	if err != nil {
		return false
	}

	if clone.Status.Code != models.StatusOK {
		return false
	}

	return true
}

// createDBLabClone creates a new clone.
func (b *Bot) createDBLabClone(ctx context.Context, user *User) (*models.Clone, error) {
	pwd, err := password.Generate(PasswordLength, PasswordMinDigits, PasswordMinSymbols, false, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate a password to a new clone")
	}

	clientRequest := dblabapi.CreateRequest{
		ID:        "joe-" + xid.New().String(),
		Project:   b.Config.ApiProject,
		Protected: false,
		DB: &dblabapi.DatabaseRequest{
			Username: joeUserNamePrefix + user.ChatUser.Name,
			Password: pwd,
		},
	}

	clone, err := b.DBLab.CreateClone(ctx, clientRequest)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a new clone")
	}

	if clone.Snapshot == nil {
		clone.Snapshot = &models.Snapshot{}
	}

	clone.DB.Password = pwd

	return clone, nil
}

// createPlatformSession starts a new platform session.
func (b *Bot) createPlatformSession(user *User, channelID string) error {
	sessionID, err := b.ApiCreatePlatformSession(user.ChatUser.ID, user.ChatUser.Name, channelID)
	if err != nil {
		log.Err("API: Create platform session:", err)

		if err := b.destroySession(user); err != nil {
			return errors.Wrap(err, "failed to stop a user session")
		}

		return errors.Wrap(err, "failed to create a platform session")
	}

	user.Session.PlatformSessionId = sessionID

	return nil
}

// destroySession destroys a DatabaseLab session.
func (b *Bot) destroySession(u *User) error {
	log.Dbg("Stopping session...")

	if err := b.DBLab.DestroyClone(context.TODO(), u.Session.Clone.ID); err != nil {
		return errors.Wrap(err, "failed to destroy clone")
	}

	b.stopSession(u)

	return nil
}

func (b *Bot) stopSession(user *User) {
	user.Session.Clone = nil
	user.Session.ConnParams = dblab.Clone{}
	user.Session.PlatformSessionId = ""

	if user.Session.CloneConnection != nil {
		user.Session.CloneConnection.Close()
	}
}

// ApiCreatePlatformSession makes an HTTP request to create a new platform session.
func (b *Bot) ApiCreatePlatformSession(uid string, username string, channel string) (string, error) {
	log.Dbg("API: Create session")

	reqData, err := json.Marshal(&api.ApiSession{
		ProjectName:   b.Config.ApiProject,
		AccessToken:   b.Config.ApiToken,
		SlackUid:      uid,
		SlackUsername: username,
		SlackChannel:  channel,
	})
	if err != nil {
		return "", err
	}

	resp, err := http.Post(b.Config.ApiUrl+"/rpc/joe_session_create",
		"application/json", bytes.NewBuffer(reqData))
	if err != nil {
		return "", err
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	respData := api.ApiCreateSessionResp{}
	err = json.Unmarshal(bodyBytes, &respData)
	if err != nil {
		return "", err
	}

	if len(respData.Code) > 0 || len(respData.Message) > 0 {
		return "", fmt.Errorf("Error: %v", respData)
	}

	log.Dbg("API: Create session success", respData.SessionId)
	return fmt.Sprintf("%d", respData.SessionId), nil
}

func appendSessionId(text string, u *User) string {
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

// TODO(anatoly): Retries, error processing.
func runMsg(msg *chatapi.Message) {
	err := msg.ChangeReaction(RCTN_RUNNING)
	if err != nil {
		log.Err(err)
	}
}

func okMsg(msg *chatapi.Message) {
	err := msg.ChangeReaction(RCTN_OK)
	if err != nil {
		log.Err(err)
	}
}

// Show bot usage hints.
func (b *Bot) showBotHints(ev *slackevents.MessageEvent, command string, query string) {
	parts := strings.SplitN(query, " ", 2)
	firstQueryWord := strings.ToLower(parts[0])

	checkQuery := len(firstQueryWord) > 0 && command == COMMAND_EXEC

	if (checkQuery && util.Contains(hintExplainDmlWords, firstQueryWord)) ||
		util.Contains(hintExplainDmlWords, command) {
		msg, _ := b.Chat.NewMessage(ev.Channel)
		err := msg.PublishEphemeral(HINT_EXPLAIN, ev.User)
		if err != nil {
			log.Err("Hint explain:", err)
		}
	}

	if util.Contains(hintExecDdlWords, command) {
		msg, _ := b.Chat.NewMessage(ev.Channel)
		err := msg.PublishEphemeral(HINT_EXEC, ev.User)
		if err != nil {
			log.Err("Hint exec:", err)
		}
	}
}

func formatSlackMessage(msg string) string {
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

func (u *User) requestQuota() error {
	limit := u.Session.QuotaLimit
	interval := u.Session.QuotaInterval
	sAgo := util.SecondsAgo(u.Session.QuotaTs)

	if sAgo < interval {
		if u.Session.QuotaCount >= limit {
			return fmt.Errorf(
				"You have reached the limit of requests per %s (%d). "+
					"Please wait before trying again.",
				english.Plural(int(interval), "second", ""),
				limit)
		}

		u.Session.QuotaCount++
		return nil
	}

	u.Session.QuotaCount = 1
	u.Session.QuotaTs = time.Now()
	return nil
}

func getForeword(idleDuration time.Duration, version, dataStateAt string) string {
	duration := durafmt.Parse(idleDuration.Round(time.Minute))
	return fmt.Sprintf(MsgSessionForewordTpl, duration, version, dataStateAt)
}
