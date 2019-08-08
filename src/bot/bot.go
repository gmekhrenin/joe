/*
2019 © Postgres.ai
*/

package bot

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"../chatapi"
	"../log"
	"../pgexplain"
	"../provision"

	_ "github.com/lib/pq"
	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
)

// TODO(anatoly): Use chat package wrapper.

const SHOW_RAW_EXPLAIN = false

const COMMAND_EXPLAIN = "explain"
const COMMAND_EXEC = "exec"
const COMMAND_SNAPSHOT = "snapshot"
const COMMAND_RESET = "reset"
const COMMAND_HARDRESET = "hardreset"
const COMMAND_HELP = "help"

var commands = []string{
	COMMAND_EXPLAIN,
	COMMAND_EXEC,
	COMMAND_SNAPSHOT,
	COMMAND_RESET,
	COMMAND_HARDRESET,
	COMMAND_HELP,
}

const QUERY_PREVIEW_SIZE = 400
const PLAN_SIZE = 1000

const MSG_HELP = "• `explain` — analyze your query (SELECT, INSERT, DELETE, UPDATE or WITH) and generate recommendations\n" +
	"• `exec` — execute any query (for example, CREATE INDEX)\n" +
	"• `snapshot` — create a snapshot of the current database state\n" +
	"• `reset` — revert the database to the initial state (usually takes less than a minute, :warning: all changes will be lost)\n" +
	"• `hardreset` — re-provision the database instance (usually takes a couple of minutes, :warning: all changes will be lost)\n" +
	"• `help` — this message"

const MSG_QUERY_REQ = "Option query required for this command, e.g. `query select 1`"

const RCTN_RUNNING = "hourglass_flowing_sand"
const RCTN_OK = "white_check_mark"
const RCTN_ERROR = "x"

const SEPARATOR_ELLIPSIS = "\n[...SKIP...]\n"
const SEPARATOR_PLAN = "\n[...SKIP...]\n"

const CUT_TEXT = "_(The text in the preview above has been cut)_"

type Audit struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	RealName string `json:"realName"`
	Command  string `json:"command"`
	Query    string `json:"query"`
}

// TODO(anatoly): verifToken should be a part of Slack API wrapper.
// TODO(anatoly): Convert args to struct.
func RunHttpServer(connStr string, port uint, chat *chatapi.Chat,
	explainConfig pgexplain.ExplainConfig, prov *provision.Provision) {

	var usersCache = make(map[string]*slack.User)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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

		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body),
			slackevents.OptionVerifyToken(
				&slackevents.TokenComparator{
					VerificationToken: chat.VerificationToken,
				}))
		if err != nil {
			log.Err("Event parse error:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Used to verified bot's API URL for Slack.
		if eventsAPIEvent.Type == slackevents.URLVerification {
			var r *slackevents.ChallengeResponse
			err := json.Unmarshal([]byte(body), &r)
			if err != nil {
				log.Err("Challenge parse error:", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text")
			w.Write([]byte(r.Challenge))
		}

		// General Slack events.
		if eventsAPIEvent.Type == slackevents.CallbackEvent {
			innerEvent := eventsAPIEvent.InnerEvent

			switch ev := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				msg, _ := chat.NewMessage(ev.Channel)
				err = msg.Publish("What's up?")
				if err != nil {
					// TODO(anatoly): Retry.
					log.Err("Bot: Cannot publish a message", err)
					return
				}
			case *slackevents.MessageEvent:
				// Skip messages sent by bots.
				if ev.User == "" || ev.BotID != "" {
					return
				}

				// Skip messages from threads.
				if ev.ThreadTimeStamp != "" {
					return
				}

				var ch = ev.Channel
				var message = strings.TrimSpace(ev.Text)

				// Get information about user.
				user, ok := usersCache[ev.User]
				if !ok {
					user, err = chat.GetUserInfo(ev.User)
					if err != nil {
						log.Err(err)

						msg, _ := chat.NewMessage(ch)
						msg.Publish(" ")
						failMsg(msg, err.Error())
						return
					}
					usersCache[ev.User] = user
				}

				// Slack escapes some characters
				// https://api.slack.com/docs/message-formatting#how_to_escape_characters
				message = strings.ReplaceAll(message, "&amp;", "&")
				message = strings.ReplaceAll(message, "&lt;", "<")
				message = strings.ReplaceAll(message, "&gt;", ">")

				// Smart quotes could be substituted automatically on macOS.
				// Replace smart quotes (“...”) with straight quotes ("...").
				message = strings.ReplaceAll(message, "“", "\"")
				message = strings.ReplaceAll(message, "”", "\"")
				message = strings.ReplaceAll(message, "‘", "'")
				message = strings.ReplaceAll(message, "’", "'")

				// Get command from snippet if exists. Snippets allow longer queries support.
				files := ev.Files
				if len(files) > 0 {
					file := files[0]
					snippet, err := chat.DownloadSnippet(file.URLPrivate)
					if err != nil {
						log.Err(err)

						msg, _ := chat.NewMessage(ch)
						msg.Publish(" ")
						failMsg(msg, err.Error())
						return
					}

					message = string(snippet)
				}

				if len(message) == 0 {
					return
				}

				// Message: "command query(optional)".
				parts := strings.SplitN(message, " ", 2)
				command := strings.ToLower(parts[0])

				query := ""
				if len(parts) > 1 {
					query = parts[1]
				}

				if !contains(commands, command) {
					return
				}

				// We want to save message height space for more valuable info.
				queryPreview := strings.ReplaceAll(query, "\n", " ")
				queryPreview = strings.ReplaceAll(queryPreview, "\t", " ")
				queryPreview, _ = cutText(queryPreview, QUERY_PREVIEW_SIZE, SEPARATOR_ELLIPSIS)

				audit, err := json.Marshal(Audit{
					Id:       user.ID,
					Name:     user.Name,
					RealName: user.RealName,
					Command:  command,
					Query:    query,
				})
				if err != nil {
					msg, _ := chat.NewMessage(ch)
					msg.Publish(" ")
					failMsg(msg, err.Error())
					return
				}
				log.Audit(string(audit))

				msg, err := chat.NewMessage(ch)
				err = msg.Publish(fmt.Sprintf("```%s %s```", command, queryPreview))
				if err != nil {
					// TODO(anatoly): Retry.
					log.Err("Bot: Cannot publish a message", err)
					return
				}

				runMsg(msg)

				switch command {
				case COMMAND_EXPLAIN:
					var detailsText string
					var trnd bool

					if query == "" {
						failMsg(msg, MSG_QUERY_REQ)
						return
					}

					// Explain request and show.
					var res, err = runQuery(connStr, "EXPLAIN (FORMAT TEXT) "+query)
					if err != nil {
						failMsg(msg, err.Error())
						return
					}

					planPreview, trnd := cutText(res, PLAN_SIZE, SEPARATOR_PLAN)

					err = msg.Append(fmt.Sprintf("*Plan:*\n```%s```", planPreview))
					if err != nil {
						log.Err("Show plan: ", err)
						failMsg(msg, err.Error())
						return
					}

					filePlanWoExec, err := chat.UploadFile("plan-wo-execution", res, ch, msg.Timestamp)
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
						return
					}

					// Explain analyze request and processing.
					res, err = runQuery(connStr,
						"EXPLAIN (ANALYZE, COSTS, VERBOSE, BUFFERS, FORMAT JSON) "+query)
					if err != nil {
						failMsg(msg, err.Error())
						return
					}

					if SHOW_RAW_EXPLAIN {
						err = msg.Append(res)
						if err != nil {
							log.Err("Show raw EXPLAIN: ", err)
							failMsg(msg, err.Error())
							return
						}
					}

					explain, err := pgexplain.NewExplain(res, explainConfig)
					if err != nil {
						log.Err("Explain parsing: ", err)
						failMsg(msg, err.Error())
						return
					}

					// Recommendations.
					tips, err := explain.GetTips()
					if err != nil {
						log.Err("Recommendations: ", err)
						failMsg(msg, err.Error())
						return
					}

					recommends := "*Recommendations:*\n"
					if len(tips) == 0 {
						recommends += ":white_check_mark: Looks good"
					} else {
						for _, tip := range tips {
							recommends += fmt.Sprintf(
								":exclamation: %s – %s <%s|Show details>\n", tip.Name,
								tip.Description, tip.DetailsUrl)
						}
					}

					err = msg.Append(recommends)
					if err != nil {
						log.Err("Show recommendations: ", err)
						failMsg(msg, err.Error())
						return
					}

					// Visualization.
					var buf = new(bytes.Buffer)
					explain.Visualize(buf)
					var vis = buf.String()

					planExecPreview, trnd := cutText(vis, PLAN_SIZE, SEPARATOR_PLAN)

					err = msg.Append(fmt.Sprintf("*EXPLAIN ANALYZE:*\n```%s```", planExecPreview))
					if err != nil {
						log.Err("Show EXPLAIN ANALYZE: ", err)
						failMsg(msg, err.Error())
						return
					}

					filePlan, err := chat.UploadFile("plan", vis, ch, msg.Timestamp)
					if err != nil {
						log.Err("File upload failed:", err)
						failMsg(msg, err.Error())
						return
					}

					detailsText = ""
					if trnd {
						detailsText = " " + CUT_TEXT
					}

					err = msg.Append(fmt.Sprintf("<%s|Full execution plan>%s", filePlan.Permalink, detailsText))
					if err != nil {
						log.Err("File: ", err)
						failMsg(msg, err.Error())
						return
					}
				case COMMAND_EXEC:
					if query == "" {
						failMsg(msg, MSG_QUERY_REQ)
						return
					}

					start := time.Now()
					var _, err = runQuery(connStr, query)
					elapsed := time.Since(start)
					if err != nil {
						log.Err("Exec:", err)
						failMsg(msg, err.Error())
						return
					}
					msg.Append(fmt.Sprintf("DDL has been executed. Execution time: %s", elapsed))
				case COMMAND_SNAPSHOT:
					if query == "" {
						failMsg(msg, MSG_QUERY_REQ)
						return
					}

					// TODO(anatoly): Refactor.
					if prov.IsLocal() {
						failMsg(msg, "`snapshot` command is not available in the current mode.")
					}

					_, err = prov.CreateZfsSnapshot(query)
					if err != nil {
						log.Err("Snapshot: ", err)
						failMsg(msg, err.Error())
						return
					}
				case COMMAND_RESET:
					msg.Append("Resetting the state of the database...")

					// TODO(anatoly): "zfs rollback" deletes newer snapshots. Users will be able
					// to jump across snapshots if we solve it.
					err := prov.ResetSession()
					if err != nil {
						log.Err("Reset:", err)
						failMsg(msg, err.Error())
						return
					}
					msg.Append("The state of the database has been reset.")
				case COMMAND_HARDRESET:
					// TODO(anatoly): Refactor
					if prov.IsLocal() {
						failMsg(msg, "`hardreset` command is not available in `local` mode.")
					}

					// Temprorary command for managing sessions.
					log.Msg("Reestablishing connection")
					msg.Append("Reestablishing connection to DB, " +
						"it may take a couple of minutes...\n" +
						"If you want to reset the state of the database use `reset` command.")

					// TODO(anatoly): Remove temporary hack.

					prov.StopSession()

					// TODO(anatoly): Temp hack. Remove after provisioning fix.
					// "Can't attach pancake drive" bug.
					time.Sleep(2 * time.Second)
					prov.StopSession()

					res, sessionId, err := prov.StartSession()
					if err != nil {
						log.Err("Hardreset:", res, sessionId, err)
						failMsg(msg, err.Error())
						return
					}
					log.Msg("Connection reestablished", res, sessionId, err)
					msg.Append("Connection reestablished")
				case COMMAND_HELP:
					msg.Append(MSG_HELP)
				}

				okMsg(msg)
			}
		}
	})

	log.Msg("Server listening on", port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	log.Err("HTTP server error:", err)
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

// Cuts length of a text if it exceeds specified size. Specifies was text cut or not.
func cutText(text string, size int, separator string) (string, bool) {
	if len(text) > size {
		size -= len(separator)
		res := text[0:size/2] + separator + text[len(text)-size/2-size%2:len(text)]
		return res, true
	}

	return text, false
}

func runQuery(connStr string, query string) (string, error) {
	log.Dbg("DB query:", query)

	// TODO(anatoly): Retry mechanic.
	var result = ""

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Err("DB connection:", err)
		return "", err
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		log.Err("DB query:", err)
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			log.Err("DB query traversal:", err)
			return s, err
		}
		result += s + "\n"
	}
	if err := rows.Err(); err != nil {
		log.Err("DB query traversal:", err)
		return result, err
	}

	return result, nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if s == item {
			return true
		}
	}
	return false
}
