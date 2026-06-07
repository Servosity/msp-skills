// Package userevents is a hand-built client for the KnowBe4 User Event API — a
// separate product from the Reporting API with its own host and its own bearer
// key (KNOWBE4_USER_EVENT_API_KEY). It lets you push custom user risk events into
// the KSAT console and read event types and request statuses.
//
// The Reporting API spec drives the rest of knowbe4-cli; the User Event API is
// modeled here because it uses a different credential and a {data,meta} response
// envelope the single-auth generated client cannot express.
//
// Outbound HTTP is paced by cliutil.AdaptiveLimiter and surfaces a typed
// *cliutil.RateLimitError when 429 retries are exhausted, so callers can tell
// "throttled" from "no data".
package userevents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"knowbe4-pp-cli/internal/cliutil"
	"net/url"
	"regexp"
)

// EnvKey is the environment variable holding the User Event API bearer token.
const EnvKey = "KNOWBE4_USER_EVENT_API_KEY"

// Client talks to the KnowBe4 User Event API.
type Client struct {
	HTTP    *http.Client
	baseURL string
	apiKey  string
	limiter *cliutil.AdaptiveLimiter
}

// regionRe bounds KNOWBE4_REGION to a hostname-safe token (knowbe4 regions
// are short lowercase codes: us, eu, ca, uk, de). Anything else would let a
// poisoned env var reshape the host the bearer key is sent to.
var regionRe = regexp.MustCompile(`^[a-z]{2,8}$`)

// BaseURLForRegion returns the User Event API host for a KnowBe4 region token.
// US has no infix (api.events.knowbe4.com); other regions use api-<region>.
// Invalid region tokens fall back to the US host (the generated Reporting
// client surfaces region misconfiguration separately via doctor).
func BaseURLForRegion(region string) string {
	r := strings.ToLower(strings.TrimSpace(region))
	if r == "" || r == "us" || !regionRe.MatchString(r) {
		return "https://api.events.knowbe4.com"
	}
	return fmt.Sprintf("https://api-%s.events.knowbe4.com", r)
}

// New builds a Client from the environment (KNOWBE4_USER_EVENT_API_KEY and the
// shared KNOWBE4_REGION). It returns an error only if the key is absent.
func New(timeout time.Duration) (*Client, error) {
	key := strings.TrimSpace(os.Getenv(EnvKey))
	if key == "" {
		return nil, fmt.Errorf("%s is not set — the User Event API uses a key separate from the Reporting API (User Event API Management Console → API key)", EnvKey)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		HTTP:    &http.Client{Timeout: timeout},
		baseURL: BaseURLForRegion(os.Getenv("KNOWBE4_REGION")),
		apiKey:  key,
		limiter: cliutil.NewAdaptiveLimiter(4.0),
	}, nil
}

// BaseURL exposes the resolved host (for diagnostics).
func (c *Client) BaseURL() string { return c.baseURL }

func (c *Client) do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}
	url := c.baseURL + path
	const maxRetries = 4
	for attempt := 0; ; attempt++ {
		c.limiter.Wait()
		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			c.limiter.OnRateLimit()
			if attempt < maxRetries {
				time.Sleep(cliutil.RetryAfter(resp))
				// body reader is consumed; non-GET retries would need a fresh
				// reader, but the User Event API's only mutating call (POST
				// /events) is safe to rebuild because body is marshaled above.
				if reqBody != nil && body != nil {
					b, _ := json.Marshal(body)
					reqBody = bytes.NewReader(b)
				}
				continue
			}
			return nil, &cliutil.RateLimitError{URL: url, RetryAfter: cliutil.RetryAfter(resp), Body: string(data)}
		}
		c.limiter.OnSuccess()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("user event API %s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
		}
		return json.RawMessage(data), nil
	}
}

// envelope unwraps the {data, meta} response shape. data may be an array or object.
type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta json.RawMessage `json:"meta"`
}

func unwrap(raw json.RawMessage) json.RawMessage {
	var e envelope
	if err := json.Unmarshal(raw, &e); err == nil && len(e.Data) > 0 {
		return e.Data
	}
	return raw
}

// ListEvents returns the user-event list (unwrapped data array).
func (c *Client) ListEvents(ctx context.Context, page, perPage int) (json.RawMessage, error) {
	return c.get(ctx, "/events", page, perPage)
}

// GetEvent returns a single user event by id.
func (c *Client) GetEvent(ctx context.Context, id string) (json.RawMessage, error) {
	raw, err := c.do(ctx, "GET", "/events/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	return unwrap(raw), nil
}

// ListEventTypes returns the configured user-event types.
func (c *Client) ListEventTypes(ctx context.Context, page, perPage int) (json.RawMessage, error) {
	return c.get(ctx, "/event_types", page, perPage)
}

// ListStatuses returns user-event request statuses.
func (c *Client) ListStatuses(ctx context.Context, page, perPage int) (json.RawMessage, error) {
	return c.get(ctx, "/statuses", page, perPage)
}

// GetStatus returns a single user-event request status by request id.
func (c *Client) GetStatus(ctx context.Context, requestID string) (json.RawMessage, error) {
	raw, err := c.do(ctx, "GET", "/statuses/"+url.PathEscape(requestID), nil)
	if err != nil {
		return nil, err
	}
	return unwrap(raw), nil
}

func (c *Client) get(ctx context.Context, path string, page, perPage int) (json.RawMessage, error) {
	q := ""
	if page > 0 {
		q = fmt.Sprintf("?page=%d", page)
		if perPage > 0 {
			q += fmt.Sprintf("&per_page=%d", perPage)
		}
	} else if perPage > 0 {
		q = fmt.Sprintf("?per_page=%d", perPage)
	}
	raw, err := c.do(ctx, "GET", path+q, nil)
	if err != nil {
		return nil, err
	}
	return unwrap(raw), nil
}

// CreateEventInput is the POST /events body. TargetUser and EventType are required.
type CreateEventInput struct {
	TargetUser     string `json:"target_user,omitempty"`
	EventType      string `json:"event_type,omitempty"`
	ExternalID     string `json:"external_id,omitempty"`
	Source         string `json:"source,omitempty"`
	Description    string `json:"description,omitempty"`
	OccurredDate   string `json:"occurred_date,omitempty"`
	RiskLevel      *int   `json:"risk_level,omitempty"`
	RiskDecayMode  string `json:"risk_decay_mode,omitempty"`
	RiskExpireDate string `json:"risk_expire_date,omitempty"`
}

// CreateEvent posts a new user event to the timeline.
func (c *Client) CreateEvent(ctx context.Context, in CreateEventInput) (json.RawMessage, error) {
	raw, err := c.do(ctx, "POST", "/events", in)
	if err != nil {
		return nil, err
	}
	return unwrap(raw), nil
}

// DeleteEvent removes a user event by id.
func (c *Client) DeleteEvent(ctx context.Context, id string) (json.RawMessage, error) {
	return c.do(ctx, "DELETE", "/events/"+url.PathEscape(id), nil)
}
