package usermanager

import (
	"fmt"
	"sync"
	"time"

	"github.com/dustin/go-humanize/english"
	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/joe/pkg/bot"
	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/structs"
	"gitlab.com/postgres-ai/joe/pkg/util"
)

type UserInformer interface {
	GetUserInfo(userID string) (structs.UserInfo, error)
}

type User struct {
	UserInfo structs.UserInfo
	Session  bot.UserSession
}

func NewUser(userInfo structs.UserInfo, cfg config.Bot) *User {
	user := User{
		UserInfo: userInfo,
		Session: bot.UserSession{
			QuotaTs:       time.Now(),
			QuotaCount:    0,
			QuotaLimit:    cfg.QuotaLimit,
			QuotaInterval: cfg.QuotaInterval,
			LastActionTs:  time.Now(),
		},
	}

	return &user
}

func (u *User) RequestQuota() error {
	limit := u.Session.QuotaLimit
	interval := u.Session.QuotaInterval
	sAgo := util.SecondsAgo(u.Session.QuotaTs)

	if sAgo < interval {
		if u.Session.QuotaCount >= limit {
			return fmt.Errorf(
				"You have reached the limit of requests per %s (%d). "+
					"Please wait before trying again.",
				english.Plural(int(interval), "second", ""),
				limit)
		}

		u.Session.QuotaCount++
		return nil
	}

	u.Session.QuotaCount = 1
	u.Session.QuotaTs = time.Now()
	return nil
}

type UserManager struct {
	UserInformer UserInformer
	Config       config.Bot

	usersMutex sync.RWMutex
	users      map[string]*User // Slack UID -> UserInfo.
}

func NewUserManager(informer UserInformer, botCfg config.Bot) UserManager {
	return UserManager{
		UserInformer: informer,
		Config:       botCfg,
		users:        make(map[string]*User),
	}
}

func (um *UserManager) CreateUser(userID string) (*User, error) {
	user, ok := um.findUser(userID)
	if ok {
		return user, nil
	}

	chatUser, err := um.UserInformer.GetUserInfo(userID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user info")
	}

	user = NewUser(chatUser, um.Config)

	if err := um.addUser(userID, user); err != nil {
		return nil, errors.Wrap(err, "failed to add user")
	}

	return user, nil
}

func (um *UserManager) findUser(userID string) (*User, bool) {
	um.usersMutex.RLock()
	user, ok := um.users[userID]
	um.usersMutex.RUnlock()

	return user, ok
}

func (um *UserManager) addUser(userID string, user *User) error {
	um.usersMutex.Lock()
	defer um.usersMutex.Unlock()

	if _, ok := um.users[userID]; ok {
		return errors.Errorf("user %q already exists", userID)
	}

	um.users[userID] = user

	return nil
}
