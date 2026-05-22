package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Issue struct {
	Key         string
	Summary     string
	Description string
}

type Client struct {
	baseURL string
	email   string
	token   string
	http    *http.Client
}

func NewClient(baseURL, email, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		email:   email,
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

type apiResp struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string `json:"summary"`
		Description any    `json:"description"` // can be string or ADF object
	} `json:"fields"`
}

func (c *Client) FetchIssue(ctx context.Context, key string) (*Issue, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", c.baseURL, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jira %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}

	return &Issue{
		Key:         r.Key,
		Summary:     r.Fields.Summary,
		Description: descToString(r.Fields.Description),
	}, nil
}

// descToString handles Jira's two description formats: legacy string
// and ADF (Atlassian Document Format) object. For ADF we just JSON-encode
// it — the LLM can still reason over it.
func descToString(d any) string {
	switch v := d.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
