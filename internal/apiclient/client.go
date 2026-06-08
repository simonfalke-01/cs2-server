// Package apiclient is a thin HTTP client for the orchestrator API, used by the
// Discord bot.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/brandonli/cs2-server/internal/model"
)

// Client talks to the orchestrator API.
type Client struct {
	base string
	http *http.Client
}

// New returns a client for the orchestrator at baseURL.
func New(baseURL string) *Client {
	return &Client{
		base: baseURL,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// InstanceView mirrors the API's instance response (instance + connect string).
type InstanceView struct {
	model.Instance
	Connect string `json:"connect"`
}

// CreateRequest is the body for creating a server.
type CreateRequest struct {
	OwnerID    string `json:"owner_id"`
	Name       string `json:"name"`
	Map        string `json:"map"`
	GameType   int    `json:"game_type"`
	GameMode   int    `json:"game_mode"`
	MaxPlayers int    `json:"max_players"`
	Public     bool   `json:"public"`
	GSLT       string `json:"gslt"`
	Password   string `json:"password"`
	BotQuota   int    `json:"bot_quota"`
}

// APIError represents a non-2xx response from the orchestrator.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("orchestrator: %d: %s", e.Status, e.Message)
}

// Create provisions a new server.
func (c *Client) Create(ctx context.Context, req CreateRequest) (*InstanceView, error) {
	var out InstanceView
	if err := c.do(ctx, http.MethodPost, "/v1/servers", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns servers, optionally filtered by owner.
func (c *Client) List(ctx context.Context, ownerID string) ([]InstanceView, error) {
	path := "/v1/servers"
	if ownerID != "" {
		path += "?owner_id=" + url.QueryEscape(ownerID)
	}
	var out []InstanceView
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Status fetches live status for a server.
func (c *Client) Status(ctx context.Context, id string) (*model.LiveStatus, error) {
	var out model.LiveStatus
	if err := c.do(ctx, http.MethodGet, "/v1/servers/"+url.PathEscape(id)+"/status", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Restart restarts a server.
func (c *Client) Restart(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/v1/servers/"+url.PathEscape(id)+"/restart", nil, nil)
}

// Stop stops and removes a server.
func (c *Client) Stop(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/servers/"+url.PathEscape(id), nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var er struct {
			Error string `json:"error"`
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(raw, &er)
		msg := er.Error
		if msg == "" {
			msg = string(raw)
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
