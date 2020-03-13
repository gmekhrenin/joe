package slack

import (
	"github.com/nlopes/slack"

	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/connection"
	"gitlab.com/postgres-ai/joe/pkg/services/dblab"
	"gitlab.com/postgres-ai/joe/pkg/services/msgproc"
	"gitlab.com/postgres-ai/joe/pkg/services/usermanager"
)

type Builder struct {
	credentialsCfg *config.Credentials
	appCfg         *config.Config
	chatApi        *slack.Client
}

func NewBuilder(slCfg *config.Credentials, cfg *config.Config, chatApi *slack.Client) *Builder {
	return &Builder{credentialsCfg: slCfg, chatApi: chatApi, appCfg: cfg}
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
