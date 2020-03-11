/*
2019 © Postgres.ai
*/

package slack

import (
	"github.com/nlopes/slack"
	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/joe/pkg/structs"
)

type UserInformer struct {
	api    *slack.Client
	config *SlackConfig
}

func NewUserInformer(api *slack.Client) *UserInformer {
	return &UserInformer{
		api: api,
	}
}

func (m *UserInformer) GetUserInfo(userID string) (structs.UserInfo, error) {
	slackUser, err := m.api.GetUserInfo(userID)
	if err != nil {
		return structs.UserInfo{}, errors.Wrap(err, "failed to get user info")
	}

	user := structs.UserInfo{
		ID:       slackUser.ID,
		Name:     slackUser.Name,
		RealName: slackUser.RealName,
	}

	return user, nil
}
