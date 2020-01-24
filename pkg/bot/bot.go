/*
2019 © Postgres.ai
*/

package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gitlab.com/postgres-ai/joe/pkg/provision"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dustin/go-humanize/english"
	"github.com/hako/durafmt"
	_ "github.com/lib/pq"
	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/sethvargo/go-password/password"

	"gitlab.com/postgres-ai/database-lab/client"
	"gitlab.com/postgres-ai/database-lab/pkg/log"
	"gitlab.com/postgres-ai/database-lab/pkg/models"
	"gitlab.com/postgres-ai/joe/pkg/chatapi"
	"gitlab.com/postgres-ai/joe/pkg/pgexplain"
	"gitlab.com/postgres-ai/joe/pkg/util"
)

const COMMAND_EXPLAIN = "explain"
const COMMAND_EXEC = "exec"
const COMMAND_SNAPSHOT = "snapshot"
const COMMAND_RESET = "reset"
const COMMAND_HARDRESET = "hardreset"
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
	COMMAND_SNAPSHOT,
	COMMAND_RESET,
	COMMAND_HARDRESET,
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
const PLAN_SIZE = 400

const IDLE_TICK_DURATION = 120 * time.Minute

const MSG_HELP = "• `explain` — analyze your query (SELECT, INSERT, DELETE, UPDATE or WITH) and generate recommendations\n" +
	"• `exec` — execute any query (for example, CREATE INDEX)\n" +
	"• `snapshot` — create a snapshot of the current database state\n" +
	"• `reset` — revert the database to the initial state (usually takes less than a minute, :warning: all changes will be lost)\n" +
	"• `hardreset` — re-provision the database instance (usually takes a couple of minutes, :warning: all changes will be lost)\n" +
	"• `\\d`, `\\d+`, `\\dt`, `\\dt+`, `\\di`, `\\di+`, `\\l`, `\\l+`, `\\dv`, `\\dv+`, `\\dm`, `\\dm+` — psql meta information commands\n" +
	"• `help` — this message\n"

const MSG_SESSION_FOREWORD_TPL = "Starting new session...\n\n" +
	"• Sessions are independent. You will have your own full-sized copy of the database.\n" +
	"• Feel free to change anything: build and drop indexes, change schema, etc.\n" +
	"• At any time, use `reset` to re-initialize the database. This will cancel the ongoing queries in your session. Say `help` to see the full list of commands.\n" +
	"• I will mark my responses with `Session: N`, where `N` is the session number (you will get your number once your session is initialized).\n" +
	"• The session will be destroyed after %s of inactivity. The corresponding DB clone will be deleted.\n" +
	"• EXPLAIN plans here are expected to be identical to production plans, essential for SQL microanalysis and optimization.\n" +
	"• The actual timing values may differ from those that production instances have because actual caches in DB Lab are smaller, therefore reading from disks is required more often. " +
	"However, the number of bytes and pages/buffers involved into query execution are the same as those on a production server.\n" +
	"\nMade with :hearts: by Postgres.ai. Bug reports, ideas, and MRs are welcome: https://gitlab.com/postgres-ai/joe \n"

var MSG_SESSION_FOREWORD = getForeword(IDLE_TICK_DURATION)

const MSG_EXEC_OPTION_REQ = "Use `exec` to run query, e.g. `exec drop index some_index_name`"
const MSG_EXPLAIN_OPTION_REQ = "Use `explain` to see the query's plan, e.g. `explain select 1`"
const MSG_SNAPSHOT_OPTION_REQ = "Use `snapshot` to create a snapshot, e.g. `snapshot state_name`"

const RCTN_RUNNING = "hourglass_flowing_sand"
const RCTN_OK = "white_check_mark"
const RCTN_ERROR = "x"

const SEPARATOR_ELLIPSIS = "\n[...SKIP...]\n"
const SEPARATOR_PLAN = "\n[...SKIP...]\n"

const CUT_TEXT = "_(The text in the preview above has been cut)_"

const HINT_EXPLAIN = "Consider using `explain` command for DML statements. See `help` for details."
const HINT_EXEC = "Consider using `exec` command for DDL statements. See `help` for details."

const dbLabUserNamePrefix = "dblab_"

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
	Config Config
	Chat   *chatapi.Chat
	DBLab  *client.Client
	Users  map[string]*User // Slack UID -> User.
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

	ChannelIds []string

	Clone *models.Clone
}

// DBLabClone contains connection info of a clone.
type DBLabClone struct {
	Name     string
	Host     string
	Port     string
	Username string
	Password string
	SSL      string
}

func (db DBLabClone) ConnectionString() string {
	return fmt.Sprintf("host=%q port=%q user=%q dbname=%q password=%q",
		db.Host, db.Port, db.Username, db.Name, db.Password)
}

func NewBot(config Config, chat *chatapi.Chat, dbLab *client.Client) *Bot {
	bot := Bot{
		Config: config,
		Chat:   chat,
		DBLab:  dbLab,
		Users:  make(map[string]*User),
	}
	return &bot
}

func NewUser(chatUser *slack.User, config Config) *User {
	user := User{
		ChatUser: chatUser,
		Session: UserSession{
			QuotaTs:       time.Now(),
			QuotaCount:    0,
			QuotaLimit:    config.QuotaLimit,
			QuotaInterval: config.QuotaInterval,
			LastActionTs:  time.Now(),
			IdleInterval:  config.IdleInterval,
		},
	}

	return &user
}

func (b *Bot) stopIdleSessions() error {
	chsNotify := make(map[string][]string)

	for _, u := range b.Users {
		if u == nil {
			continue
		}

		s := u.Session
		if s.Clone == nil {
			continue
		}

		interval := u.Session.IdleInterval
		sAgo := util.SecondsAgo(u.Session.LastActionTs)

		if sAgo < interval {
			continue
		}

		log.Dbg("Session idle: %v %v", u, s)

		for _, ch := range u.Session.ChannelIds {
			uId := u.ChatUser.ID
			chNotify, ok := chsNotify[ch]
			if !ok {
				chsNotify[ch] = []string{uId}
				continue
			}

			chsNotify[ch] = append(chNotify, uId)
		}

		b.stopSession(u)
	}

	// Publish message in every channel with a list of users.
	for ch, uIds := range chsNotify {
		if len(uIds) == 0 {
			continue
		}

		list := ""
		for _, uId := range uIds {
			if len(list) > 0 {
				list += ", "
			}
			list += fmt.Sprintf("<@%s>", uId)
		}

		msgText := "Stopped idle sessions for: " + list

		msg, _ := b.Chat.NewMessage(ch)
		err := msg.Publish(msgText)
		if err != nil {
			log.Err("Bot: Cannot publish a message", err)
		}
	}

	return nil
}

func (b *Bot) stopAllSessions() error {
	for _, u := range b.Users {
		if u == nil || u.Session.Clone == nil {
			continue
		}

		if err := b.stopSession(u); err != nil {
			log.Errf("Failed to stop session %q: %v\n", u.Session.Clone.ID, err)
			continue
		}
	}

	return nil
}

func (b *Bot) stopSession(u *User) error {
	log.Dbg("Stopping session...")

	if err := b.DBLab.DestroyClone(context.TODO(), u.Session.Clone.ID); err != nil {
		return errors.Wrap(err, "failed to destroy clone")
	}

	u.Session.Clone = nil
	u.Session.PlatformSessionId = ""

	return nil
}

func (b *Bot) RunServer() {
	// Stop idle sessions.
	_ = util.RunInterval(IDLE_TICK_DURATION, func() {
		log.Dbg("Stop idle sessions tick")
		b.stopIdleSessions()
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

	explainConfig := b.Config.Explain

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
	user, ok := b.Users[ev.User]
	if !ok {
		chatUser, err := b.Chat.GetUserInfo(ev.User)
		if err != nil {
			log.Err(err)

			msg, _ := b.Chat.NewMessage(ch)
			msg.Publish(" ")
			failMsg(msg, err.Error())
			return
		}

		user = NewUser(chatUser, b.Config)
		b.Users[ev.User] = user
	}
	user.Session.LastActionTs = time.Now()
	if !util.Contains(user.Session.ChannelIds, ch) {
		user.Session.ChannelIds = append(user.Session.ChannelIds, ch)
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
			failMsg(msg, err.Error())
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
	command := strings.ToLower(parts[0])

	query := ""
	if len(parts) > 1 {
		query = parts[1]
	}

	b.showBotHints(ev, command, query)

	if !util.Contains(supportedCommands, command) {
		log.Dbg("Message filtered: Not a command")
		return
	}

	err = user.requestQuota()
	if err != nil {
		log.Err("Quota: ", err)
		msg, _ := b.Chat.NewMessage(ch)
		msg.Publish(" ")
		failMsg(msg, err.Error())
		return
	}

	// We want to save message height space for more valuable info.
	queryPreview := strings.ReplaceAll(query, "\n", " ")
	queryPreview = strings.ReplaceAll(queryPreview, "\t", " ")
	queryPreview, _ = cutText(queryPreview, QUERY_PREVIEW_SIZE, SEPARATOR_ELLIPSIS)

	audit, err := json.Marshal(Audit{
		Id:       user.ChatUser.ID,
		Name:     user.ChatUser.Name,
		RealName: user.ChatUser.RealName,
		Command:  command,
		Query:    query,
	})
	if err != nil {
		msg, _ := b.Chat.NewMessage(ch)
		msg.Publish(" ")
		failMsg(msg, err.Error())
		return
	}
	log.Audit(string(audit))

	msgText := fmt.Sprintf("```%s %s```\n", command, queryPreview)

	// Show `help` command without initializing of a session.
	if command == COMMAND_HELP {
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

	if user.Session.Clone == nil {
		sMsg, _ := b.Chat.NewMessage(ch)
		sMsg.Publish(MSG_SESSION_FOREWORD)

		runMsg(sMsg)

		pwd, err := password.Generate(16, 4, 4, false, true)
		if err != nil {
			failMsg(sMsg, err.Error())
			return
		}

		clientRequest := client.CreateRequest{
			Name:      xid.New().String(),
			Project:   user.Session.PlatformSessionId,
			Protected: false,
			DB: &client.DatabaseRequest{
				Username: dbLabUserNamePrefix + user.ChatUser.Name,
				Password: pwd,
			},
		}

		//session, err := b.Prov.StartSession()
		clone, err := b.DBLab.CreateClone(context.TODO(), clientRequest)
		if err != nil {
			failMsg(sMsg, err.Error())
			return
		}

		time.Sleep(3 * time.Second) // TODO(akartasov): Make synchronous API request.
		clone, err = b.DBLab.GetClone(context.TODO(), clone.ID)
		if err != nil {
			failMsg(sMsg, err.Error())
			return
		}

		user.Session.Clone = clone
		user.Session.Clone.Db.Password = pwd // TODO(akartasov): Should keep a password?

		if b.Config.HistoryEnabled {
			sId, err := b.ApiCreateSession(user.ChatUser.ID, user.ChatUser.Name, ch)
			if err != nil {
				log.Err("API: Create platform session:", err)

				b.stopSession(user)
				failMsg(sMsg, err.Error())
				return
			}

			user.Session.PlatformSessionId = sId
		}

		// clone ID
		sId := clone.ID
		if user.Session.PlatformSessionId != "" {
			sId = user.Session.PlatformSessionId
		}

		sMsg.Append(fmt.Sprintf("Session started: `%s`", sId))
		okMsg(sMsg)
	}

	msgText = appendSessionId(msgText, user)

	dbLabClone := DBLabClone{
		Name:     b.Config.DBLab.DBName,
		Host:     user.Session.Clone.Db.Host,
		Port:     user.Session.Clone.Db.Port,
		Username: user.Session.Clone.Db.Username,
		Password: user.Session.Clone.Db.Password,
		SSL:      b.Config.DBLab.SSLMode,
	}

	connStr := dbLabClone.ConnectionString()
	log.Dbg(connStr)

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

	apiCmd := &ApiCommand{
		SessionId: user.Session.PlatformSessionId,
		Command:   command,
		Query:     query,
		SlackTs:   ev.TimeStamp,
		Error:     "",
	}

	switch {
	case command == COMMAND_EXPLAIN:
		var detailsText string
		var trnd bool

		if query == "" {
			failMsg(msg, MSG_EXPLAIN_OPTION_REQ)
			b.failApiCmd(apiCmd, MSG_EXPLAIN_OPTION_REQ)
			return
		}

		// Explain request and show.
		var res, err = dbExplain(connStr, query)
		if err != nil {
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		apiCmd.PlanText = res
		planPreview, trnd := cutText(res, PLAN_SIZE, SEPARATOR_PLAN)

		msgInitText := msg.Text

		err = msg.Append(fmt.Sprintf("*Plan:*\n```%s```", planPreview))
		if err != nil {
			log.Err("Show plan: ", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		filePlanWoExec, err := b.Chat.UploadFile("plan-wo-execution-text", res, ch, msg.Timestamp)
		if err != nil {
			log.Err("File upload failed:", err)
			failMsg(msg, err.Error())
			return
		}

		detailsText = ""
		if trnd {
			detailsText = " " + CUT_TEXT
		}

		err = msg.Append(fmt.Sprintf("<%s|Full plan (w/o execution)>%s", filePlanWoExec.Permalink, detailsText))
		if err != nil {
			log.Err("File: ", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		// Explain analyze request and processing.
		res, err = dbExplainAnalyze(connStr, query)
		if err != nil {
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		apiCmd.PlanExecJson = res

		// Visualization.
		explain, err := pgexplain.NewExplain(res, explainConfig)
		if err != nil {
			log.Err("Explain parsing: ", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		vis := explain.RenderPlanText()
		apiCmd.PlanExecText = vis

		planExecPreview, trnd := cutText(vis, PLAN_SIZE, SEPARATOR_PLAN)

		err = msg.Replace(msgInitText + chatapi.CHAT_APPEND_SEPARATOR +
			fmt.Sprintf("*Plan with execution:*\n```%s```", planExecPreview))
		if err != nil {
			log.Err("Show the plan with execution:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		_, err = b.Chat.UploadFile("plan-json", res, ch, msg.Timestamp)
		if err != nil {
			log.Err("File upload failed:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		filePlan, err := b.Chat.UploadFile("plan-text", vis, ch, msg.Timestamp)
		if err != nil {
			log.Err("File upload failed:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		detailsText = ""
		if trnd {
			detailsText = " " + CUT_TEXT
		}

		err = msg.Append(fmt.Sprintf("<%s|Full execution plan>%s \n"+
			"_Other artifacts are provided in the thread_",
			filePlan.Permalink, detailsText))
		if err != nil {
			log.Err("File: ", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		// Recommendations.
		tips, err := explain.GetTips()
		if err != nil {
			log.Err("Recommendations: ", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		recommends := ""
		if len(tips) == 0 {
			recommends += ":white_check_mark: Looks good"
		} else {
			for _, tip := range tips {
				recommends += fmt.Sprintf(
					":exclamation: %s – %s <%s|Show details>\n", tip.Name,
					tip.Description, tip.DetailsUrl)
			}
		}

		apiCmd.Recommendations = recommends

		err = msg.Append("*Recommendations:*\n" + recommends)
		if err != nil {
			log.Err("Show recommendations: ", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		// Summary.
		stats := explain.RenderStats()
		apiCmd.Stats = stats

		err = msg.Append(fmt.Sprintf("*Summary:*\n```%s```", stats))
		if err != nil {
			log.Err("Show summary: ", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

	case command == COMMAND_EXEC:
		if query == "" {
			failMsg(msg, MSG_EXEC_OPTION_REQ)
			b.failApiCmd(apiCmd, MSG_EXEC_OPTION_REQ)
			return
		}

		start := time.Now()
		err = dbExec(connStr, query)
		elapsed := time.Since(start)
		if err != nil {
			log.Err("Exec:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		duration := util.DurationToString(elapsed)
		result := fmt.Sprintf("The query has been executed. Duration: %s", duration)
		apiCmd.Response = result

		err = msg.Append(result)
		if err != nil {
			log.Err("Exec:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

	case command == COMMAND_SNAPSHOT:
		if query == "" {
			failMsg(msg, MSG_SNAPSHOT_OPTION_REQ)
			b.failApiCmd(apiCmd, MSG_SNAPSHOT_OPTION_REQ)
			return
		}

		// TODO(akartasov): Add method to API.
		//err = b.Prov.CreateSnapshot(query)
		err = errors.New("an unsupported command for now")
		if err != nil {
			log.Err("Snapshot: ", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

	case command == COMMAND_RESET:
		msg.Append("Resetting the state of the database...")

		// TODO(anatoly): "zfs rollback" deletes newer snapshots. Users will be able
		// to jump across snapshots if we solve it.
		//err = b.DBLabInstance.ResetSession(user.Session.Provision)
		err = b.DBLab.ResetClone(context.TODO(), user.Session.Clone.ID)
		if err != nil {
			log.Err("Reset:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		result := "The state of the database has been reset."
		apiCmd.Response = result

		err = msg.Append(result)
		if err != nil {
			log.Err("Reset:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

	case command == COMMAND_HARDRESET:
		log.Msg("Reinitilizating provision")
		msg.Append("Reinitilizating DB provision, " +
			"it may take a couple of minutes...\n" +
			"If you want to reset the state of the database use `reset` command.")

		if err := b.stopAllSessions(); err != nil {
			log.Err("Hardreset:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		result := "Provision reinitilized"
		log.Msg(result, err)

		apiCmd.Response = result

		err = msg.Append(result)
		if err != nil {
			log.Err("Hardreset:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

	case util.Contains(allowedPsqlCommands, command):
		// See provision.runPsqlStrict for more comments.
		if strings.ContainsAny(query, "\n;\\ ") {
			err = fmt.Errorf("Query should not contain semicolons, " +
				"new lines, spaces, and excess backslashes.")
			log.Err(err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		psqlCmd := command + " " + query

		// TODO(akartasov): Keep psql for psql commands available to users - runPsqlStrict
		runner := provision.NewSqlRunner(
			user.Session.Clone.Db.ConnStr,
			user.Session.Clone.Db.Password,
			provision.LogsEnabledDefault,
		)
		cmd, err := provision.RunPsqlStrict(runner, psqlCmd)
		if err != nil {
			log.Err(err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		apiCmd.Response = cmd

		cmdPreview, trnd := cutText(cmd, PLAN_SIZE, SEPARATOR_PLAN)

		err = msg.Append(fmt.Sprintf("*Command output:*\n```%s```", cmdPreview))
		if err != nil {
			log.Err("Show psql output:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		fileCmd, err := b.Chat.UploadFile("command", cmd, ch, msg.Timestamp)
		if err != nil {
			log.Err("File upload failed:", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}

		detailsText := ""
		if trnd {
			detailsText = " " + CUT_TEXT
		}

		err = msg.Append(fmt.Sprintf("<%s|Full command output>%s\n", fileCmd.Permalink, detailsText))
		if err != nil {
			log.Err("File: ", err)
			failMsg(msg, err.Error())
			b.failApiCmd(apiCmd, err.Error())
			return
		}
	}

	if b.Config.HistoryEnabled {
		_, err := b.ApiPostCommand(apiCmd)
		if err != nil {
			log.Err(err)
			failMsg(msg, err.Error())
			return
		}
	}

	okMsg(msg)
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

func failMsg(msg *chatapi.Message, text string) {
	err := msg.Append(fmt.Sprintf("ERROR: %s", text))
	if err != nil {
		log.Err(err)
	}

	err = msg.ChangeReaction(RCTN_ERROR)
	if err != nil {
		log.Err(err)
	}
}

func (b *Bot) failApiCmd(apiCmd *ApiCommand, text string) {
	apiCmd.Error = text
	_, err := b.ApiPostCommand(apiCmd)
	if err != nil {
		log.Err("failApiCmd:", err)
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

// Cuts length of a text if it exceeds specified size. Specifies was text cut or not.
func cutText(text string, size int, separator string) (string, bool) {
	if len(text) > size {
		size -= len(separator)
		res := text[0:size] + separator
		return res, true
	}

	return text, false
}

func getForeword(idleDuration time.Duration) string {
	duration := durafmt.Parse(idleDuration.Round(time.Minute))
	return fmt.Sprintf(MSG_SESSION_FOREWORD_TPL, duration)
}
