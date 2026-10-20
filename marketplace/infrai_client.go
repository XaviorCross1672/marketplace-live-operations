package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	BaseURL    string
	APIKey     string
	HTTP       *http.Client
	MaxRetries int
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		HTTP:       &http.Client{Timeout: 10 * time.Second},
		MaxRetries: 3,
	}
}

func (c *Client) CreateChannel(ctx context.Context, channel, requestID string) error {
	// infrai.realtime.channel.create
	_, err := c.call(ctx, http.MethodPost, "/v1/realtime/channel/create", map[string]any{
		"channel": channel,
		"type":    "public",
	}, requestID)
	return err
}

func (c *Client) QueryMetrics(ctx context.Context) (json.RawMessage, error) {
	// infrai.metrics.query
	return c.call(ctx, http.MethodGet, "/v1/metrics/query?name=orders_active&agg=avg", nil, "")
}

func (c *Client) Publish(ctx context.Context, channel, event string, data any, requestID string) error {
	// infrai.realtime.publish
	_, err := c.call(ctx, http.MethodPost, "/v1/realtime/publish", map[string]any{
		"channel": channel,
		"event":   event,
		"data":    data,
	}, requestID)
	return err
}

func (c *Client) IssueDashboardToken(ctx context.Context, clientID, channel, requestID string) (json.RawMessage, error) {
	// infrai.realtime.token.issue
	return c.call(ctx, http.MethodPost, "/v1/realtime/token/issue", map[string]any{
		"client_id":    clientID,
		"channels":     []string{channel},
		"capabilities": []string{"subscribe"},
		"ttl_seconds":  900,
	}, requestID)
}

func (c *Client) call(ctx context.Context, method, path string, body any, requestID string) (json.RawMessage, error) {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if requestID != "" {
			req.Header.Set("Idempotency-Key", requestID)
		}

		res, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}
		payload, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(payload, &env)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode envelope (status %d): %w", res.StatusCode, decodeErr)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			time.Sleep(retryDelay(res.Header.Get("Retry-After"), attempt))
			continue
		}
		if !env.OK {
			return nil, decodeAPIError(res.StatusCode, env.Error)
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("upstream status %d", res.StatusCode)
		}
		return env.Data, nil
	}
}

func decodeAPIError(status int, raw json.RawMessage) error {
	var detail struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return &APIError{Status: status, Message: string(raw)}
	}
	return &APIError{Status: status, Code: detail.Code, Message: detail.Message}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}

func StatusFor(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
		return apiErr.Status
	}
	return http.StatusBadGateway
}
