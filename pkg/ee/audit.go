/*
2019 © Postgres.ai
*/

// Package ee provides the Enterprise features.
package ee

type Audit struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	RealName string `json:"realName"`
	Command  string `json:"command"`
	Query    string `json:"query"`
}
