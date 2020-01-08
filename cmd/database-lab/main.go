/*
2019 © Postgres.ai
*/

// TODO(anatoly):
// - Validate configs in all components.
// - Pass username and password and set it additionally to main username/password.
// - Tests.
// - CI: Gofmt, lint, misspell.
// - Graceful shutdown.
// - Don't kill clones on shutdown/start.

package main

import (
	"bytes"

	"gitlab.com/postgres-ai/database-lab/pkg/config"
	"gitlab.com/postgres-ai/database-lab/pkg/log"
	"gitlab.com/postgres-ai/database-lab/pkg/services/cloning"
	"gitlab.com/postgres-ai/database-lab/pkg/services/provision"
	"gitlab.com/postgres-ai/database-lab/pkg/srv"

	"github.com/jessevdk/go-flags"
)

var opts struct {
	VerificationToken string `short:"v" long:"verification-token" description:"callback URL verification token" env:"VERIFICATION_TOKEN"`
	DbPassword        string `description:"database password" env:"DB_PASSWORD" default:"postgres"`

	ShowHelp func() error `long:"help" description:"Show this help message"`
}

func main() {
	// Load CLI options.
	var _, err = parseArgs()

	if err != nil {
		if flags.WroteHelp(err) {
			return
		}

		log.Fatal("Args parse error:", err)
		return
	}

	log.DEBUG = true

	cfg, err := config.LoadConfig("config.yml")
	if err != nil {
		log.Fatal("Config parse error:", err)
		return
	}

	log.Dbg("Config loaded", cfg)

	if len(opts.DbPassword) > 0 {
		cfg.Provision.DbPassword = opts.DbPassword
	}

	provisionSvc, err := provision.NewProvision(cfg.Provision)
	if err != nil {
		log.Fatal("Error in \"provision\" config:", err)
		return
	}

	cloningSvc, err := cloning.NewCloning(&cfg.Cloning, provisionSvc)
	if err != nil {
		log.Fatal(err)
		return
	}

	if err = cloningSvc.Run(); err != nil {
		log.Fatal(err)
		return
	}

	if len(opts.VerificationToken) > 0 {
		cfg.Server.VerificationToken = opts.VerificationToken
	}

	server := srv.NewServer(&cfg.Server, cloningSvc)
	if err = server.Run(); err != nil {
		log.Fatal(err)
	}
}

func parseArgs() ([]string, error) {
	var parser = flags.NewParser(&opts, flags.Default & ^flags.HelpFlag)

	// jessevdk/go-flags lib doesn't allow to use short flag -h because
	// it's binded to usage help. We need to hack it a bit to use -h
	// for as a hostname option.
	// See https://github.com/jessevdk/go-flags/issues/240
	opts.ShowHelp = func() error {
		var b bytes.Buffer

		parser.WriteHelp(&b)
		return &flags.Error{
			Type:    flags.ErrHelp,
			Message: b.String(),
		}
	}

	return parser.Parse()
}