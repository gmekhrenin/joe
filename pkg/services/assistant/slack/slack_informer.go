package slack

import (
	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/joe/pkg/structs"
)

func (m *Messenger) GetUserInfo(userID string) (structs.UserInfo, error) {
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
