/*
2019 © Postgres.ai
*/

package config

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"

	"gitlab.com/postgres-ai/joe/pkg/pgexplain"
)

// Bot defines an App configuration.
type Bot struct {
	Version                  string
	Port                     uint
	Explain                  pgexplain.ExplainConfig
	Quota                    Quota
	AuditEnabled             bool
	MinNotifyDurationMinutes uint
	DBLab                    DBLabInstance
	Platform                 Platform
	Connections              MessengerConfig          `yaml:"connections"`
	DBLabInstances           map[string]DBLabInstance `yaml:"dblabs"`
}

// Quota contains quota configuration parameters.
type Quota struct {
	Limit    uint
	Interval uint // Seconds.
}

// DBLabInstance contains Database Lab config.
type DBLabInstance struct {
	URL     string `yaml:"url"`
	Token   string `yaml:"token"`
	DBName  string `yaml:"dbname"`
	SSLMode string `yaml:"sslmode"`
}

// Platform describes configuration parameters of a Postgres.ai platform.
type Platform struct {
	URL            string
	Token          string
	Project        string
	HistoryEnabled bool
}

// MessengerConfig defines available connections.
type MessengerConfig struct {
	Slack []Workspace `yaml:"slack,flow"`
}

// Workspace defines a Slack workspaces.
type Workspace struct {
	Name     string         `yaml:"name"`
	Channels []SlackChannel `yaml:"channels"`
}

// SlackChannel defines a Slack channel configuration.
type SlackChannel struct {
	ChannelID string `yaml:"channelID"`
	DBLabID   string `yaml:"dblab"`
}

// Load loads configuration from file.
func Load(filename string) (*Bot, error) {
	//nolint:gosec
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Bot
	if err = yaml.Unmarshal(bytes, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
