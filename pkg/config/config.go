/*
2019 © Postgres.ai
*/

// Package config provides the App configuration.
package config

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"

	"gitlab.com/postgres-ai/joe/pkg/pgexplain"
)

// Config defines an App configuration.
type Config struct {
	App                      App
	Version                  string
	Port                     uint
	Explain                  pgexplain.ExplainConfig
	Quota                    Quota
	MinNotifyDurationMinutes uint
	Platform                 Platform
	ChannelMapping           *ChannelMapping
}

// App defines a general application configuration.
type App struct {
	Version                  string
	Port                     uint
	AuditEnabled             bool
	MinNotifyDurationMinutes uint
	MaxDBLabInstances        uint
}

// Quota contains quota configuration parameters.
type Quota struct {
	Limit    uint
	Interval uint // Seconds.
}

// Platform describes configuration parameters of a Postgres.ai platform.
type Platform struct {
	URL            string
	Token          string
	Project        string
	HistoryEnabled bool
}

// ChannelMapping contains configuration parameters of communication types and Database Labs.
type ChannelMapping struct {
	CommunicationTypes map[string][]Workspace   `yaml:"communicationTypes,flow"`
	DBLabInstances     map[string]DBLabInstance `yaml:"dblabServers"`
}

// DBLabInstance contains Database Lab config.
type DBLabInstance struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

// Workspace defines a connection space.
type Workspace struct {
	Name        string      `yaml:"name"`
	Credentials Credentials `yaml:"credentials"`
	Channels    []Channel   `yaml:"channels"`
}

// Credentials defines connection space credentials.
type Credentials struct {
	AccessToken   string `yaml:"accessToken"`
	SigningSecret string `yaml:"signingSecret"`
}

// Channel defines a connection channel configuration.
type Channel struct {
	ChannelID   string      `yaml:"channelID" json:"channel_id"`
	DBLabID     string      `yaml:"dblabServer" json:"-"`
	DBLabParams DBLabParams `yaml:"dblabParams" json:"-"`
}

// DBLabParams defines database params for clone creation.
type DBLabParams struct {
	DBName  string `yaml:"dbname" json:"-"`
	SSLMode string `yaml:"sslmode" json:"-"`
}

// Load loads configuration from file.
func Load(filename string) (*ChannelMapping, error) {
	//nolint:gosec
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config ChannelMapping
	if err = yaml.Unmarshal(bytes, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
