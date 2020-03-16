/*
2019 © Postgres.ai
*/

package slack

import (
	"github.com/nlopes/slack"
	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/services/dblab"
	"gitlab.com/postgres-ai/joe/pkg/services/msgproc"
	"gitlab.com/postgres-ai/joe/pkg/services/usermanager"
)

// Builder describes a Slack assistant builder.
type Builder struct {
	credentialsCfg *config.Credentials
	appCfg         *config.Config
	chatApi        *slack.Client
}

// NewBuilder creates a new builder.
func NewBuilder(credentials *config.Credentials, cfg *config.Config, chatApi *slack.Client) (*Builder, error) {
	if err := validateCredentials(credentials); err != nil {
		return nil, errors.Wrap(err, "invalid credentials given")
	}

	return &Builder{credentialsCfg: credentials, chatApi: chatApi, appCfg: cfg}, nil
}

func (b *Builder) Build(dbLabInstance *dblab.DBLabInstance) connection.Assistant {
	slackCfg := &SlackConfig{
		AccessToken:   b.credentialsCfg.AccessToken,
		SigningSecret: b.credentialsCfg.SigningSecret,
	}

	messenger := NewMessenger(b.chatApi, slackCfg)
	userInformer := NewUserInformer(b.chatApi)
	userManager := usermanager.NewUserManager(userInformer, b.appCfg.Quota)

	processingCfg := msgproc.ProcessingConfig{
		App:      b.appCfg.App,
		Platform: b.appCfg.Platform,
		Explain:  b.appCfg.Explain,
		DBLab:    dbLabInstance.Config(),
	}

	processingService := msgproc.NewProcessingService(messenger, MessageValidator{}, dbLabInstance.Client(), userManager, processingCfg)

	return NewAssistant(b.credentialsCfg, processingService)
}

func validateCredentials(credentials *config.Credentials) error {
	if credentials == nil || credentials.AccessToken == "" || credentials.SigningSecret == "" {
		return errors.New("access_token and signing_secret must not be empty")
	}

	return nil
}
