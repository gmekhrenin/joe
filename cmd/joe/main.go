/*
Joe Bot

2019 © Postgres.ai

Conversational UI bot for Postgres query optimization.
*/

package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/jessevdk/go-flags"
	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/bot"
	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/ee/command/builder"
	"gitlab.com/postgres-ai/joe/pkg/ee/options"
)

var opts struct {
	// HTTP Server.
	ServerPort uint `short:"s" long:"http-port" description:"HTTP server port" env:"SERVER_PORT" default:"3001"`

	MinNotifyDuration uint `long:"min-notify-duration" description:"a time interval (in minutes) to notify a user about the finish of a long query" env:"MIN_NOTIFY_DURATION" default:"1"`

	// Platform.
	PlatformURL     string `long:"api-url" description:"Postgres.ai platform API base URL" env:"API_URL" default:"https://postgres.ai/api/general"` // nolint:lll
	PlatformToken   string `long:"api-token" description:"Postgres.ai platform API token" env:"API_TOKEN"`
	PlatformProject string `long:"api-project" description:"Postgres.ai platform project to assign user sessions" env:"API_PROJECT"`
	HistoryEnabled  bool   `long:"history-enabled" description:"send command and queries history to Postgres.ai platform for collaboration and visualization" env:"HISTORY_ENABLED"` // nolint:lll

	// Dev.
	DevGitCommitHash string `long:"git-commit-hash" env:"GIT_COMMIT_HASH" default:""`
	DevGitBranch     string `long:"git-branch" env:"GIT_BRANCH" default:""`
	DevGitModified   bool   `long:"git-modified" env:"GIT_MODIFIED"`

	Debug bool `long:"debug" description:"Enable a debug mode"`

	ShowHelp func() error `long:"help" description:"Show this help message"`

	// Enterprise features (changing these options you confirm that you have active subscription to Postgres.ai Platform Enterprise Edition https://postgres.ai).
	Enterprise options.Enterprise `group:"Enterprise Options" env-namespace:"EE"`
}

// TODO (akartasov): Set the app version during build.
const Version = "v0.6.1-rc1"

// TODO(anatoly): Refactor configs and envs.

func main() {
	// Load CLI options.
	var _, err = parseArgs()

	if err != nil {
		if flags.WroteHelp(err) {
			return
		}

		log.Err("Args parse error", err)
		return
	}

	log.DEBUG = opts.Debug

	// Load and validate configuration files.
	explainConfig, err := config.LoadExplainConfig()
	if err != nil {
		log.Err("Unable to load explain config", err)
		return
	}

	log.Dbg("Explain config loaded", explainConfig)

	version := formatBotVersion(opts.DevGitCommitHash, opts.DevGitBranch, opts.DevGitModified)

	log.Dbg("git: ", version)

	spaceCfg, err := config.Load("config/config.yml")
	if err != nil {
		log.Fatal(err)
	}

	botCfg := config.Config{
		App: config.App{
			Version:                  version,
			Port:                     opts.ServerPort,
			AuditEnabled:             opts.Enterprise.AuditEnabled,
			MinNotifyDurationMinutes: opts.MinNotifyDuration,
		},
		Explain: explainConfig,
		Quota: config.Quota{
			Limit:    opts.Enterprise.QuotaLimit,
			Interval: opts.Enterprise.QuotaInterval,
		},

		Platform: config.Platform{
			URL:            opts.PlatformURL,
			Token:          opts.PlatformToken,
			Project:        opts.PlatformProject,
			HistoryEnabled: opts.HistoryEnabled,
		},
	}

	enterprise := bot.NewEnterprise(builder.NewBuilder())

	joeBot := bot.NewApp(botCfg, spaceCfg, enterprise)
	if err := joeBot.RunServer(context.Background()); err != nil {
		log.Err("HTTP server error:", err)
	}
}

func parseArgs() ([]string, error) {
	var optParser = flags.NewParser(&opts, flags.Default & ^flags.HelpFlag)

	// jessevdk/go-flags lib doesn't allow to use short flag -h because it's binded to usage help.
	// We need to hack it a bit to use -h for as a hostname option. See https://github.com/jessevdk/go-flags/issues/240
	opts.ShowHelp = func() error {
		var b bytes.Buffer

		optParser.WriteHelp(&b)
		return &flags.Error{
			Type:    flags.ErrHelp,
			Message: b.String(),
		}
	}

	return optParser.Parse()
}

func formatBotVersion(commit string, branch string, modified bool) string {
	if len(commit) < 7 {
		return Version
	}

	modifiedStr := ""
	if modified {
		modifiedStr = " (modified)"
	}

	commitShort := commit[:7]

	return fmt.Sprintf("%s@%s%s", commitShort, branch, modifiedStr)
}
