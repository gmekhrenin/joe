/*
2019 © Postgres.ai
*/

package bot

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/nlopes/slack"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gitlab.com/postgres-ai/database-lab/pkg/client/dblabapi"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	slackConnection "gitlab.com/postgres-ai/joe/pkg/connection/slack"
	"gitlab.com/postgres-ai/joe/pkg/services/dblab"
	"gitlab.com/postgres-ai/joe/pkg/util"
)

// InactiveCloneCheckInterval defines an interval for check of idleness sessions.
const InactiveCloneCheckInterval = time.Minute

// Workspace types
const (
	slackWorkspace = "slack"
)

// App defines a application struct.
type App struct {
	Config   config.Config
	spaceCfg *config.Space

	dblabMu        *sync.RWMutex
	dblabInstances map[string]*dblab.DBLabInstance
}

// Creates a new application.
func NewApp(cfg config.Config, spaceCfg *config.Space) *App {
	bot := App{
		Config:         cfg,
		spaceCfg:       spaceCfg,
		dblabMu:        &sync.RWMutex{},
		dblabInstances: make(map[string]*dblab.DBLabInstance, len(spaceCfg.DBLabInstances)),
	}

	return &bot
}

// RunServer starts a server for message processing.
func (a *App) RunServer(ctx context.Context) error {
	if err := a.initDBLabInstances(); err != nil {
		return errors.Wrap(err, "failed to init Database Lab instances")
	}

	assistants, err := a.getAllAssistants()
	if err != nil {
		return errors.Wrap(err, "failed to get application assistants")
	}

	for _, assistantSvc := range assistants {
		if err := assistantSvc.Register(); err != nil {
			return errors.Wrap(err, "failed to init an assistant")
		}

		// Check idle sessions.
		_ = util.RunInterval(InactiveCloneCheckInterval, func() {
			log.Dbg("Check idle sessions")
			assistantSvc.CheckIdleSessions(ctx)
		})
	}

	port := a.Config.App.Port
	log.Msg(fmt.Sprintf("Server start listening on localhost:%d", port))

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		return errors.Wrap(err, "failed to start a server")
	}

	return nil
}

func (a *App) initDBLabInstances() error {
	for name, dbLab := range a.spaceCfg.DBLabInstances {
		dbLabClient, err := dblabapi.NewClient(dblabapi.Options{
			Host:              dbLab.URL,
			VerificationToken: dbLab.Token,
		}, logrus.New())

		if err != nil {
			return errors.Wrap(err, "failed to create a Database Lab client")
		}

		a.dblabMu.Lock()
		a.dblabInstances[name] = dblab.NewDBLabInstance(dbLabClient, dbLab)
		a.dblabMu.Unlock()
	}

	return nil
}

func (a *App) getAllAssistants() ([]connection.Assistant, error) {
	assistants := []connection.Assistant{}

	for workspaceType, workspaceList := range a.spaceCfg.Connections {
		for _, workspace := range workspaceList {
			workspaceAssistants, err := a.getWorkspaceAssistants(workspaceType, workspace)
			if err != nil {
				return nil, errors.Wrap(err, "failed to register workspace assistants")
			}

			assistants = append(assistants, workspaceAssistants...)
		}
	}

	return assistants, nil
}

func (a *App) getWorkspaceAssistants(workspaceType string, workspace config.Workspace) ([]connection.Assistant, error) {
	builder, err := a.getAssistantBuilder(workspaceType, workspace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get an assistant builder")
	}

	assistants := make([]connection.Assistant, 0, len(workspace.Channels))

	for _, channel := range workspace.Channels {
		a.dblabMu.RLock()

		dbLabInstance, ok := a.dblabInstances[channel.DBLabID]
		if !ok {
			a.dblabMu.RUnlock()
			return nil, errors.Errorf("Failed to find a configuration of the Database Lab client: %q", channel.DBLabID)
		}

		a.dblabMu.RUnlock()

		assistant := builder.Build(dbLabInstance)
		assistant.SetHandlerPrefix(fmt.Sprintf("/%s/%s", workspaceType, channel.ChannelID))

		assistants = append(assistants, assistant)
	}

	return assistants, nil
}

func (a *App) getAssistantBuilder(workspaceType string, workspaceCfg config.Workspace) (connection.AssistantBuiilder, error) {
	switch workspaceType {
	case slackWorkspace:
		chatAPI := slack.New(workspaceCfg.Credentials.AccessToken)
		return slackConnection.NewBuilder(&workspaceCfg.Credentials, &a.Config, chatAPI), nil

	default:
		return nil, errors.New("unknown workspace type given")
	}
}
