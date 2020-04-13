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

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/jessevdk/go-flags"
	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/features"
	"gitlab.com/postgres-ai/joe/features/definition"
	"gitlab.com/postgres-ai/joe/pkg/bot"
	"gitlab.com/postgres-ai/joe/pkg/config"
)

var opts struct {
	DevGitCommitHash string `long:"git-commit-hash" env:"GIT_COMMIT_HASH" default:""`
	DevGitBranch     string `long:"git-branch" env:"GIT_BRANCH" default:""`
	DevGitModified   bool   `long:"git-modified" env:"GIT_MODIFIED"`

	ShowHelp func() error `long:"help" description:"Show this help message"`
}

// TODO (akartasov): Set the app version during build.
const Version = "v0.7.0"

var buildTime string

// TODO(anatoly): Refactor configs and envs.

func main() {
	enterpriseFlagProvider := features.GetFlagProvider()

	// Load CLI options.
	if _, err := parseArgs(enterpriseFlagProvider); err != nil {
		if flags.WroteHelp(err) {
			return
		}

		log.Fatal("Args parse error", err)
	}

	// Load and validate configuration files.
	explainConfig, err := config.LoadExplainConfig()
	if err != nil {
		log.Fatal("Unable to load explain config", err)
	}

	var botCfg config.Config

	if err := cleanenv.ReadConfig("config/config.yml", &botCfg); err != nil {
		log.Fatal(err)
	}

	log.DEBUG = botCfg.App.Debug

	version := formatBotVersion(opts.DevGitCommitHash, opts.DevGitBranch, opts.DevGitModified)

	log.Dbg("git: ", version)

	botCfg.App.Version = version
	botCfg.Explain = explainConfig
	botCfg.EnterpriseOptions = enterpriseFlagProvider.ToOpts()

	joeBot := bot.NewApp(botCfg, features.NewPack())
	if err := joeBot.RunServer(context.Background()); err != nil {
		log.Err("HTTP server error:", err)
	}
}

func parseArgs(ent definition.FlagProvider) ([]string, error) {
	var optParser = flags.NewParser(&opts, flags.Default & ^flags.HelpFlag)

	entGroup, err := optParser.AddGroup("Enterprise Options",
		"Available only for Postgres.ai Platform Enterprise Edition https://postgres.ai", ent)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init Enterprise options")
	}

	entGroup.EnvNamespace = "EE"

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
		return Version + "-" + buildTime
	}

	modifiedStr := ""
	if modified {
		modifiedStr = " (modified)"
	}

	commitShort := commit[:7]

	return fmt.Sprintf("%s@%s%s", commitShort, branch, modifiedStr)
}
