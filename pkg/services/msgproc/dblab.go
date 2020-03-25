/*
2019 © Postgres.ai
*/

// Package msgproc provides a service for processing of incoming events.
package msgproc

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hako/durafmt"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/sethvargo/go-password/password"

	"gitlab.com/postgres-ai/database-lab/pkg/client/dblabapi/types"
	"gitlab.com/postgres-ai/database-lab/pkg/log"
	dblabmodels "gitlab.com/postgres-ai/database-lab/pkg/models"

	"gitlab.com/postgres-ai/joe/pkg/models"
	"gitlab.com/postgres-ai/joe/pkg/services/platform"
	"gitlab.com/postgres-ai/joe/pkg/services/usermanager"
)

// HelpMessage defines available commands provided with the help message.
const HelpMessage = "• `explain` — analyze your query (SELECT, INSERT, DELETE, UPDATE or WITH) and generate recommendations\n" +
	"• `plan` — analyze your query (SELECT, INSERT, DELETE, UPDATE or WITH) without execution\n" +
	"• `exec` — execute any query (for example, CREATE INDEX)\n" +
	"• `reset` — revert the database to the initial state (usually takes less than a minute, :warning: all changes will be lost)\n" +
	"• `\\d`, `\\d+`, `\\dt`, `\\dt+`, `\\di`, `\\di+`, `\\l`, `\\l+`, `\\dv`, `\\dv+`, `\\dm`, `\\dm+` — psql meta information commands\n" +
	"• `hypo` — create hypothetical indexes using the HypoPG extension\n" +
	"• `help` — this message\n"

// MsgSessionStarting provides a message for a session start.
const MsgSessionStarting = "Starting new session...\n"

// MsgSessionForewordTpl provides a template of session foreword message.
const MsgSessionForewordTpl = "• Say 'help' to see the full list of commands.\n" +
	"• Sessions are fully independent. Feel free to do anything.\n" +
	"• The session will be destroyed after %s of inactivity.\n" +
	"• EXPLAIN plans here are expected to be identical to production plans.\n" +
	"• The actual timing values may differ from production because actual caches in DB Lab are smaller. " +
	"However, the number of bytes and pages/buffers in plans are identical to production.\n" +
	"\nMade with :hearts: by Postgres.ai. Bug reports, ideas, and merge requests are welcome: https://gitlab.com/postgres-ai/joe \n" +
	"\nJoe version: %s.\nSnapshot data state at: %s."

// SeparatorEllipsis provides a separator for cut messages.
const SeparatorEllipsis = "\n[...SKIP...]\n"

// QueryPreviewSize defines a max preview size of query in message.
const QueryPreviewSize = 400

// Hint messages.
const (
	HintExplain = "Consider using `explain` command for DML statements. See `help` for details."
	HintExec    = "Consider using `exec` command for DDL statements. See `help` for details."
)

const (
	joeUserNamePrefix = "joe_"
	joeSessionPrefix  = "joe-"
)

// Constants for autogenerated passwords.
const (
	PasswordLength     = 16
	PasswordMinDigits  = 4
	PasswordMinSymbols = 0
)

var hintExplainDmlWords = []string{"insert", "select", "update", "delete", "with"}
var hintExecDdlWords = []string{"alter", "create", "drop", "set"}

// runSession starts a user session if not exists.
func (s *ProcessingService) runSession(ctx context.Context, user *usermanager.User, incomingMessage models.IncomingMessage) error {
	sMsg := models.NewMessage(incomingMessage)

	messageText := strings.Builder{}

	if user.Session.Clone != nil {
		return nil
	}

	// Stop clone session if not active.
	s.stopSession(user)

	messageText.WriteString(MsgSessionStarting)
	sMsg.SetText(messageText.String())
	s.messenger.Publish(sMsg)
	messageText.Reset()

	s.messenger.UpdateStatus(sMsg, models.StatusRunning)

	sessionID := user.Session.PlatformSessionID

	if sessionID == "" {
		if incomingMessage.SessionID != "" {
			sessionID = incomingMessage.SessionID
		} else {
			sessionID = generateSessionID()
		}
	}

	clone, err := s.createDBLabClone(ctx, user, sessionID)
	if err != nil {
		s.messenger.Fail(sMsg, err.Error())

		return err
	}

	sMsg.AppendText(getForeword(time.Duration(clone.Metadata.MaxIdleMinutes)*time.Minute,
		s.config.App.Version, clone.Snapshot.DataStateAt))

	if err := s.messenger.UpdateText(sMsg); err != nil {
		s.messenger.Fail(sMsg, err.Error())
		return errors.Wrap(err, "failed to append message with a foreword")
	}

	if clone.DB == nil {
		return errors.New("failed to get connection params")
	}

	dblabClone := s.buildDBLabCloneConn(clone.DB)

	db, err := initConn(dblabClone)
	if err != nil {
		return errors.Wrap(err, "failed to init database connection")
	}

	user.Session.ConnParams = dblabClone
	user.Session.Clone = clone
	user.Session.CloneConnection = db

	if s.config.Platform.HistoryEnabled && user.Session.PlatformSessionID == "" {
		if err := s.createPlatformSession(ctx, user, sMsg.ChannelID); err != nil {
			s.messenger.Fail(sMsg, err.Error())
			return err
		}

		user.Session.PlatformSessionID = sessionID
	}

	sMsg.AppendText(fmt.Sprintf("Session started: `%s`", sessionID))

	if err := s.messenger.UpdateText(sMsg); err != nil {
		s.messenger.Fail(sMsg, err.Error())
		return errors.Wrap(err, "failed to append message about session start")
	}

	if err := s.messenger.OK(sMsg); err != nil {
		log.Err(err)
	}

	return nil
}

func (s *ProcessingService) buildDBLabCloneConn(dbParams *dblabmodels.Database) models.Clone {
	return models.Clone{
		Name:     s.config.DBLab.DBName,
		Host:     dbParams.Host,
		Port:     dbParams.Port,
		Username: dbParams.Username,
		Password: dbParams.Password,
		SSLMode:  s.config.DBLab.SSLMode,
	}
}

func initConn(dblabClone models.Clone) (*sql.DB, error) {
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

// createDBLabClone creates a new clone.
func (s *ProcessingService) createDBLabClone(ctx context.Context, user *usermanager.User, sessionID string) (*dblabmodels.Clone, error) {
	pwd, err := password.Generate(PasswordLength, PasswordMinDigits, PasswordMinSymbols, false, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate a password to a new clone")
	}

	clientRequest := types.CloneCreateRequest{
		ID:        sessionID,
		Project:   s.config.Platform.Project,
		Protected: false,
		DB: &types.DatabaseRequest{
			Username: joeUserNamePrefix + user.UserInfo.Name,
			Password: pwd,
		},
	}

	clone, err := s.DBLab.CreateClone(ctx, clientRequest)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a new clone")
	}

	if clone.Snapshot == nil {
		clone.Snapshot = &dblabmodels.Snapshot{}
	}

	clone.DB.Password = pwd

	return clone, nil
}

// createPlatformSession starts a new platform session.
func (s *ProcessingService) createPlatformSession(ctx context.Context, user *usermanager.User, channelID string) error {
	platformSession := platform.Session{
		ProjectName: s.config.Platform.Project,
		UserID:      user.UserInfo.ID,
		Username:    user.UserInfo.Name,
		ChannelID:   channelID,
	}

	if _, err := s.platformManager.CreatePlatformSession(ctx, platformSession); err != nil {
		log.Err("API: Create platform session:", err)

		if err := s.destroySession(user); err != nil {
			return errors.Wrap(err, "failed to stop a user session")
		}

		return errors.Wrap(err, "failed to create a platform session")
	}

	return nil
}

// generateSessionID generates a session ID for Joe.
func generateSessionID() string {
	return joeSessionPrefix + xid.New().String()
}

func getForeword(idleDuration time.Duration, version, dataStateAt string) string {
	duration := durafmt.Parse(idleDuration.Round(time.Minute))
	return fmt.Sprintf(MsgSessionForewordTpl, duration, version, dataStateAt)
}
