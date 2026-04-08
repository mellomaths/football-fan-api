// Package apiclient is an HTTP client for the Football Fan API.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client calls the REST API.
type Client struct {
	baseURL    string
	apiKey     string
	log        *slog.Logger
	httpClient *http.Client
}

// New returns a client for baseURL (no trailing slash). apiKey is sent as X-API-Key on mutating routes.
// log may be nil; when set, requests and failures are logged for debugging.
func New(baseURL, apiKey string, log *slog.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		log:     log,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

// Team mirrors GET /teams items.
type Team struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	ShortName     string  `json:"short_name,omitempty"`
	EspnSlug      string  `json:"espn_slug,omitempty"`
	SoccerwayID   string  `json:"soccerway_id,omitempty"`
	TicketSaleURL *string `json:"ticket_sale_url,omitempty"`
}

// Subscriber is POST /users response.
type Subscriber struct {
	ID             int64           `json:"id"`
	ExternalKey    string          `json:"external_key"`
	DeliveryTarget string          `json:"delivery_target"`
	DisplayName    *string         `json:"display_name,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

// SubscriptionRow is GET /users/subscriptions item.
type SubscriptionRow struct {
	SubscriberID   int64  `json:"subscriber_id"`
	DeliveryTarget string `json:"delivery_target"`
	TeamID         int64  `json:"team_id"`
}

// Match mirrors GET /teams/{id}/matches items.
type Match struct {
	ID          int64          `json:"id"`
	KickoffUTC  string         `json:"kickoff_utc"`
	Location    *MatchLocation `json:"location,omitempty"`
	Home        TeamMatchSide  `json:"home"`
	Away        TeamMatchSide  `json:"away"`
	Competition CompetitionRef `json:"competition"`
}

// MatchLocation is the venue name for a fixture.
type MatchLocation struct {
	Name string `json:"name"`
}

// TeamMatchSide is a side in a match.
type TeamMatchSide struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// CompetitionRef identifies the competition.
type CompetitionRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// TicketAnnouncement mirrors GET ticket announcements.
type TicketAnnouncement struct {
	ID               int64  `json:"id"`
	SaleScheduleText string `json:"sale_schedule_text"`
	PricesText       string `json:"prices_text"`
	ScrapedAt        string `json:"scraped_at"`
	Match            *Match `json:"match"`
}

// ListTeams calls GET /teams with optional name filter.
func (c *Client) ListTeams(ctx context.Context, name string) ([]Team, error) {
	path := "/teams"
	if name != "" {
		path = "/teams?name=" + url.QueryEscape(name)
	}
	var out []Team
	if err := c.get(ctx, path, &out); err != nil {
		if c.log != nil {
			c.log.Warn("list teams request failed", slog.String("name_filter", name), slog.Any("err", err))
		}
		return nil, err
	}
	if out == nil {
		out = []Team{}
	}
	if c.log != nil {
		c.log.Info("list teams response",
			slog.String("name_filter", name),
			slog.Int("team_count", len(out)),
		)
	}
	return out, nil
}

// GetTeam calls GET /teams/{id}.
func (c *Client) GetTeam(ctx context.Context, teamID int64) (Team, error) {
	var out Team
	err := c.get(ctx, "/teams/"+strconv.FormatInt(teamID, 10), &out)
	return out, err
}

// ListMatches calls GET /teams/{id}/matches with date range YYYY-MM-DD.
func (c *Client) ListMatches(ctx context.Context, teamID int64, from, to string) ([]Match, error) {
	q := url.Values{}
	q.Set("from", from)
	q.Set("to", to)
	path := "/teams/" + strconv.FormatInt(teamID, 10) + "/matches?" + q.Encode()
	var out []Match
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Match{}
	}
	return out, nil
}

// ListTicketAnnouncements calls GET /teams/{id}/tickets/announcements with RFC3339 from/to.
func (c *Client) ListTicketAnnouncements(ctx context.Context, teamID int64, from, to time.Time) ([]TicketAnnouncement, error) {
	q := url.Values{}
	q.Set("from", from.UTC().Format(time.RFC3339))
	q.Set("to", to.UTC().Format(time.RFC3339))
	path := "/teams/" + strconv.FormatInt(teamID, 10) + "/tickets/announcements?" + q.Encode()
	var out []TicketAnnouncement
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []TicketAnnouncement{}
	}
	return out, nil
}

type upsertUserBody struct {
	ExternalKey    string          `json:"external_key"`
	DeliveryTarget string          `json:"delivery_target"`
	DisplayName    *string         `json:"display_name,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

// UpsertUser calls POST /users (requires API key).
func (c *Client) UpsertUser(ctx context.Context, externalKey, deliveryTarget string, displayName *string, metadata json.RawMessage) (Subscriber, error) {
	body := upsertUserBody{
		ExternalKey:    externalKey,
		DeliveryTarget: deliveryTarget,
		DisplayName:    displayName,
		Metadata:       metadata,
	}
	var out Subscriber
	err := c.postJSON(ctx, "/users", body, &out, true)
	return out, err
}

type subscriptionBody struct {
	TeamID int64 `json:"team_id"`
}

// AddSubscription calls POST /users/{id}/subscription.
func (c *Client) AddSubscription(ctx context.Context, userID, teamID int64) error {
	path := "/users/" + strconv.FormatInt(userID, 10) + "/subscription"
	return c.postJSON(ctx, path, subscriptionBody{TeamID: teamID}, nil, true)
}

// ListSubscriptions calls GET /users/subscriptions.
func (c *Client) ListSubscriptions(ctx context.Context) ([]SubscriptionRow, error) {
	var out []SubscriptionRow
	err := c.getAuth(ctx, "/users/subscriptions", &out)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []SubscriptionRow{}
	}
	return out, nil
}

// PostNotificationReceipt calls POST /notification-receipts; returns whether the row was newly inserted.
func (c *Client) PostNotificationReceipt(ctx context.Context, subscriberID, announcementID int64) (inserted bool, err error) {
	body := map[string]int64{
		"subscriber_id":   subscriberID,
		"announcement_id": announcementID,
	}
	var out struct {
		Inserted bool `json:"inserted"`
	}
	if err := c.postJSON(ctx, "/notification-receipts", body, &out, true); err != nil {
		return false, err
	}
	return out.Inserted, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out, false)
}

func (c *Client) getAuth(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out, true)
}

func (c *Client) postJSON(ctx context.Context, path string, body any, out any, auth bool) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out, auth)
}

func (c *Client) do(req *http.Request, out any, auth bool) error {
	if auth {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	if c.log != nil {
		c.log.Debug("api request",
			slog.String("method", req.Method),
			slog.String("url", req.URL.String()),
			slog.Bool("auth_header", auth),
		)
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		if c.log != nil {
			c.log.Warn("api transport error", slog.String("url", req.URL.String()), slog.Any("err", err))
		}
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body := bytes.TrimSpace(b)
		if c.log != nil {
			c.log.Warn("api error response",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.Int("status", res.StatusCode),
				slog.String("body", truncateForLog(body, 768)),
			)
		}
		return fmt.Errorf("api %s %s: status %d: %s", req.Method, req.URL.Path, res.StatusCode, body)
	}
	if out == nil || len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		if c.log != nil {
			c.log.Warn("api json decode error",
				slog.String("url", req.URL.String()),
				slog.Any("err", err),
				slog.String("body_prefix", truncateForLog(bytes.TrimSpace(b), 512)),
			)
		}
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

func truncateForLog(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
