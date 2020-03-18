/*
2019 © Postgres.ai
*/

// Package platform provides a Platform API client.
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"gitlab.com/postgres-ai/database-lab/pkg/log"

	"gitlab.com/postgres-ai/joe/pkg/config"
	"gitlab.com/postgres-ai/joe/pkg/models"
)

const (
	accessToken = "Access-Token"
)

// Command represents an incoming command and its results.
type Command struct {
	SessionID string `json:"session_id"`

	Command  string `json:"command"`
	Query    string `json:"query"`
	Response string `json:"response"`

	// Explain.
	PlanText        string `json:"plan_text"`
	PlanJSON        string `json:"plan_json"`
	PlanExecText    string `json:"plan_execution_text"`
	PlanExecJSON    string `json:"plan_execution_json"`
	Recommendations string `json:"recommendations"`
	Stats           string `json:"stats"`

	Error string `json:"error"`

	Timestamp string `json:"slack_ts"`
}

// Client provides a Platform API client.
type Client struct {
	url         *url.URL
	accessToken string
	client      *http.Client
	cfg         config.Platform
}

// NewClient creates a new Platform API client.
func NewClient(platformCfg config.Platform) (*Client, error) {
	u, err := url.Parse(platformCfg.URL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse a platform host")
	}

	u.Path = strings.TrimRight(u.Path, "/")

	p := Client{
		url:         u,
		accessToken: platformCfg.Token,
		client: &http.Client{
			Transport: &http.Transport{},
		},
	}

	return &p, nil
}

func (p *Client) doRequest(ctx context.Context, request *http.Request, parser responseParser) error {
	request.Header.Add(accessToken, p.accessToken)
	request = request.WithContext(ctx)

	response, err := p.client.Do(request)
	if err != nil {
		return errors.Wrap(err, "failed to make a request")
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return errors.Errorf("unsuccessful status given: %d", response.StatusCode)
	}

	return parser(response)
}

// PostCommand makes an HTTP request to post a command to Platform.
func (p *Client) PostCommand(ctx context.Context, command *Command) (string, error) {
	reqData, err := json.Marshal(command)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal a command")
	}

	postURL := p.buildURL("/rpc/joe_session_command_post").String()

	r, err := http.NewRequest(http.MethodPost, postURL, bytes.NewBuffer(reqData))
	if err != nil {
		return "", errors.Wrap(err, "failed to create a command request")
	}

	commandResp := PostCommandResponse{}
	if err := p.doRequest(ctx, r, newJSONParser(&commandResp)); err != nil {
		return "", err
	}

	if commandResp.Code != "" || commandResp.Message != "" {
		return "", errors.Errorf("Error: %v", commandResp)
	}

	log.Dbg("API: Post command success", commandResp.CommandID)

	return strconv.FormatUint(uint64(commandResp.CommandID), 10), nil
}

// CreatePlatformSession makes an HTTP request to create a new Platform session.
func (p *Client) CreatePlatformSession(ctx context.Context, session Session) (string, error) {
	log.Dbg("API: Create session")

	session.ProjectName = p.cfg.Project

	reqData, err := json.Marshal(session)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal a command")
	}

	postURL := p.buildURL("/rpc/joe_session_create").String()

	r, err := http.NewRequest(http.MethodPost, postURL, bytes.NewBuffer(reqData))
	if err != nil {
		return "", errors.Wrap(err, "failed to create a command request")
	}

	respData := CreateSessionResponse{}

	if err := p.doRequest(ctx, r, newJSONParser(&respData)); err != nil {
		return "", errors.Wrap(err, "failed to do request")
	}

	if respData.Code != "" || respData.Message != "" {
		return "", errors.Errorf("error: %v", respData)
	}

	log.Dbg("API: Create session success", respData.SessionID)

	return strconv.FormatUint(uint64(respData.SessionID), 10), nil
}

// PostMessage makes an HTTP request to post a new message to Platform.
func (p *Client) PostMessage(ctx context.Context, message *models.Message) (string, error) {
	reqData, err := json.Marshal(message)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal a message")
	}

	postURL := p.buildURL("/rpc/joe_message_post").String()

	r, err := http.NewRequest(http.MethodPost, postURL, bytes.NewBuffer(reqData))
	if err != nil {
		return "", errors.Wrap(err, "failed to create a request")
	}

	respData := PostMessageResponse{}

	if err := p.doRequest(ctx, r, newJSONParser(&respData)); err != nil {
		return "", errors.Wrap(err, "failed to do request")
	}

	if respData.Code != "" || respData.Message != "" {
		return "", errors.Errorf("error: %v", respData)
	}

	log.Dbg("Platform API: Message has been successfully created", respData.MessageID)

	return strconv.FormatUint(uint64(respData.MessageID), 10), nil
}

// ArtifactUploadParameters represents parameters to upload artifact to Platform.
type ArtifactUploadParameters struct {
	MessageID string `json:"message_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
}

// AddArtifact makes an HTTP request to upload an artifact to Platform.
func (p *Client) AddArtifact(ctx context.Context, uploadParameters ArtifactUploadParameters) (string, error) {
	reqData, err := json.Marshal(uploadParameters)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal upload parameters")
	}

	postURL := p.buildURL("/rpc/joe_message_artifact_post").String()

	r, err := http.NewRequest(http.MethodPost, postURL, bytes.NewBuffer(reqData))
	if err != nil {
		return "", errors.Wrap(err, "failed to create a request")
	}

	respData := AddArtifactResponse{}

	if err := p.doRequest(ctx, r, newJSONParser(&respData)); err != nil {
		return "", errors.Wrap(err, "failed to do request")
	}

	if respData.Code != "" || respData.Message != "" {
		return "", errors.Errorf("error: %v", respData)
	}

	log.Dbg("Platform API: Artifact has been successfully uploaded", respData.ArtifactLink)

	return respData.ArtifactLink, nil
}

// URL builds URL for a specific endpoint.
func (p *Client) buildURL(urlPath string) *url.URL {
	fullPath := path.Join(p.url.Path, urlPath)

	u := *p.url
	u.Path = fullPath

	return &u
}

type responseParser func(*http.Response) error

func newJSONParser(v interface{}) responseParser {
	return func(resp *http.Response) error {
		return json.NewDecoder(resp.Body).Decode(v)
	}
}

// Session represent a Platform session.
type Session struct {
	ProjectName string `json:"project_name"`
	AccessToken string `json:"access_token"`
	UserID      string `json:"slack_uid"`
	Username    string `json:"slack_username"`
	ChannelID   string `json:"slack_channel"`
}

// APIResponse represents common fields of an API response.
type APIResponse struct {
	Hint    string `json:"hint"`
	Details string `json:"details"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CreateSessionResponse represents a response of a session creating request.
type CreateSessionResponse struct {
	APIResponse
	SessionID uint `json:"session_id"`
}

// PostCommandResponse represents a response of a posting command request.
type PostCommandResponse struct {
	APIResponse
	CommandID uint `json:"command_id"`
}

// PostMessageResponse represents a response of a posting message request.
type PostMessageResponse struct {
	APIResponse
	MessageID uint `json:"message_id"`
}

// AddArtifactResponse represents a response of an artifact uploading.
type AddArtifactResponse struct {
	APIResponse
	ArtifactLink string `json:"link"`
}
