// Package httpapi implements the HTTP API for teams and matches.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mellomaths/football-fan-api/api/internal/db"
	"github.com/mellomaths/football-fan-api/api/internal/validate"
)

// Store is the read model used by HTTP handlers (implemented by *db.Store).
type Store interface {
	ListTeams(ctx context.Context, nameQuery string) ([]db.Team, error)
	GetTeamByID(ctx context.Context, teamID int64) (db.Team, error)
	PatchTeamTicketSaleURL(ctx context.Context, teamID int64, patch db.TeamTicketSalePatch) (db.Team, error)
	TeamExists(ctx context.Context, teamID int64) (bool, error)
	ListMatchesForTeam(ctx context.Context, teamID int64, fromInclusive, toExclusive time.Time) ([]db.Match, error)
	ListTicketAnnouncementsForTeam(ctx context.Context, sellerTeamID int64, fromInclusive, toInclusive time.Time) ([]db.TicketAnnouncement, error)
	UpsertSubscriber(ctx context.Context, externalKey, deliveryTarget string, displayName *string, metadata json.RawMessage) (db.Subscriber, error)
	GetSubscriberByID(ctx context.Context, id int64) (db.Subscriber, error)
	AddSubscriberTeamSubscription(ctx context.Context, subscriberID, teamID int64) error
	ListSubscriberSubscriptions(ctx context.Context) ([]db.SubscriptionRow, error)
	TryInsertNotificationReceipt(ctx context.Context, subscriberID, announcementID int64) (inserted bool, err error)
}

// Server wires HTTP handlers to the data store.
type Server struct {
	log            *slog.Logger
	store          Store
	internalAPIKey string
}

// NewServer returns an API server. internalAPIKey, when non-empty, is required as X-API-Key for subscriber routes.
func NewServer(log *slog.Logger, store Store, internalAPIKey string) *Server {
	return &Server{log: log, store: store, internalAPIKey: internalAPIKey}
}

// Handler returns the root mux with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /teams", s.handleTeams)
	mux.HandleFunc("GET /teams/{teamId}/tickets/announcements", s.handleTeamTicketAnnouncements)
	mux.HandleFunc("GET /teams/{teamId}/matches", s.handleTeamMatches)
	mux.HandleFunc("GET /teams/{teamId}", s.handleTeamByID)
	mux.HandleFunc("PATCH /teams/{teamId}", s.handlePatchTeam)
	mux.HandleFunc("POST /users", s.requireInternalAuth(s.handlePostUsers))
	mux.HandleFunc("POST /users/{userId}/subscription", s.requireInternalAuth(s.handlePostUserSubscription))
	mux.HandleFunc("GET /users/subscriptions", s.requireInternalAuth(s.handleGetUserSubscriptions))
	mux.HandleFunc("POST /notification-receipts", s.requireInternalAuth(s.handlePostNotificationReceipt))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}

func (s *Server) requireInternalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.internalAPIKey == "" {
			s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "internal subscriber API is not configured (set API_INTERNAL_KEY)",
			})
			return
		}
		if r.Header.Get("X-API-Key") != s.internalAPIKey {
			s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

type postUserRequest struct {
	ExternalKey    string          `json:"external_key"`
	DeliveryTarget string          `json:"delivery_target"`
	DisplayName    *string         `json:"display_name"`
	Metadata       json.RawMessage `json:"metadata"`
}

func (s *Server) handlePostUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var body postUserRequest
	if err := dec.Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(body.ExternalKey) == "" || strings.TrimSpace(body.DeliveryTarget) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "external_key and delivery_target are required"})
		return
	}
	ctx := r.Context()
	out, err := s.store.UpsertSubscriber(ctx, strings.TrimSpace(body.ExternalKey), strings.TrimSpace(body.DeliveryTarget), body.DisplayName, body.Metadata)
	if err != nil {
		s.log.Error("upsert subscriber", slog.Any("err", err))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	s.writeJSON(w, http.StatusOK, out)
}

type postSubscriptionRequest struct {
	TeamID int64 `json:"team_id"`
}

func (s *Server) handlePostUserSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	userStr := r.PathValue("userId")
	subscriberID, err := strconv.ParseInt(userStr, 10, 64)
	if err != nil || subscriberID < 1 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var body postSubscriptionRequest
	if decodeErr := dec.Decode(&body); decodeErr != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.TeamID < 1 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "team_id is required"})
		return
	}
	ctx := r.Context()
	if _, getErr := s.store.GetSubscriberByID(ctx, subscriberID); getErr != nil {
		if errors.Is(getErr, db.ErrSubscriberNotFound) {
			s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		s.log.Error("get subscriber", slog.Any("err", getErr))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	ok, err := s.store.TeamExists(ctx, body.TeamID)
	if err != nil {
		s.log.Error("team exists", slog.Any("err", err))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
		return
	}
	if err := s.store.AddSubscriberTeamSubscription(ctx, subscriberID, body.TeamID); err != nil {
		if errors.Is(err, db.ErrSubscriptionDuplicate) {
			s.writeJSON(w, http.StatusConflict, map[string]string{"error": "already subscribed to this team"})
			return
		}
		s.log.Error("add subscription", slog.Any("err", err))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	s.writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) handleGetUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	ctx := r.Context()
	rows, err := s.store.ListSubscriberSubscriptions(ctx)
	if err != nil {
		s.log.Error("list subscriber subscriptions", slog.Any("err", err))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if rows == nil {
		rows = []db.SubscriptionRow{}
	}
	s.writeJSON(w, http.StatusOK, rows)
}

type postNotificationReceiptRequest struct {
	SubscriberID   int64 `json:"subscriber_id"`
	AnnouncementID int64 `json:"announcement_id"`
}

func (s *Server) handlePostNotificationReceipt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var body postNotificationReceiptRequest
	if err := dec.Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.SubscriberID < 1 || body.AnnouncementID < 1 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subscriber_id and announcement_id are required"})
		return
	}
	ctx := r.Context()
	inserted, err := s.store.TryInsertNotificationReceipt(ctx, body.SubscriberID, body.AnnouncementID)
	if err != nil {
		s.log.Error("notification receipt", slog.Any("err", err))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"inserted": inserted})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, err := w.Write([]byte(`{"status":"ok"}`))
	if err != nil {
		s.log.Error("healthz write", slog.Any("err", err))
	}
}

func (s *Server) handleTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	teams, err := s.store.ListTeams(ctx, name)
	if err != nil {
		s.log.Error("list teams", slog.String("name_filter", name), slog.Any("err", err))
		s.writeJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{"error": "internal server error"},
		)
		return
	}
	if teams == nil {
		teams = []db.Team{}
	}
	s.log.Info("list teams",
		slog.String("name_filter", name),
		slog.Int("team_count", len(teams)),
	)
	if name != "" && len(teams) == 0 {
		s.log.Warn("list teams returned no rows for name filter — only teams linked via team_competitions appear in GET /teams; raw teams rows without a competition link are excluded",
			slog.String("name_filter", name),
		)
	}
	s.writeJSON(w, http.StatusOK, teams)
}

func (s *Server) handleTeamByID(w http.ResponseWriter, r *http.Request) {
	teamStr := r.PathValue("teamId")
	teamID, err := strconv.ParseInt(teamStr, 10, 64)
	if err != nil || teamID < 1 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid team id"})
		return
	}
	ctx := r.Context()
	detail, err := s.store.GetTeamByID(ctx, teamID)
	if err != nil {
		if errors.Is(err, db.ErrTeamNotFound) {
			s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
			return
		}
		s.log.Error("get team", slog.Any("err", err))
		s.writeJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{"error": "internal server error"},
		)
		return
	}
	s.writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handlePatchTeam(w http.ResponseWriter, r *http.Request) {
	teamStr := r.PathValue("teamId")
	teamID, err := strconv.ParseInt(teamStr, 10, 64)
	if err != nil || teamID < 1 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid team id"})
		return
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	var body map[string]json.RawMessage
	if err = dec.Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	var patch db.TeamTicketSalePatch
	raw, hasTicket := body["ticket_sale_url"]
	if hasTicket {
		patch.TicketSaleURLSet = true
		switch string(raw) {
		case "null":
			patch.TicketSaleURL = nil
		default:
			var urlStr string
			if err = json.Unmarshal(raw, &urlStr); err != nil {
				s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ticket_sale_url must be a string or null"})
				return
			}
			var norm string
			norm, err = validate.TicketSaleURL(urlStr)
			if err != nil {
				s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			patch.TicketSaleURL = &norm
		}
	}
	ctx := r.Context()
	out, err := s.store.PatchTeamTicketSaleURL(ctx, teamID, patch)
	if err != nil {
		if errors.Is(err, db.ErrTeamNotFound) {
			s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
			return
		}
		s.log.Error("patch team", slog.Any("err", err))
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTeamTicketAnnouncements(w http.ResponseWriter, r *http.Request) {
	teamStr := r.PathValue("teamId")
	teamID, err := strconv.ParseInt(teamStr, 10, 64)
	if err != nil || teamID < 1 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid team id"})
		return
	}
	ctx := r.Context()
	ok, err := s.store.TeamExists(ctx, teamID)
	if err != nil {
		s.log.Error("team exists", slog.Any("err", err))
		s.writeJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{"error": "internal server error"},
		)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
		return
	}
	q := r.URL.Query()
	ir, err := validate.ParseRequiredInstantRange(q.Get("from"), q.Get("to"))
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ann, err := s.store.ListTicketAnnouncementsForTeam(ctx, teamID, ir.FromInclusive, ir.ToInclusive)
	if err != nil {
		s.log.Error("list ticket announcements", slog.Any("err", err))
		s.writeJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{"error": "internal server error"},
		)
		return
	}
	if ann == nil {
		ann = []db.TicketAnnouncement{}
	}
	s.writeJSON(w, http.StatusOK, ann)
}

func (s *Server) handleTeamMatches(w http.ResponseWriter, r *http.Request) {
	teamStr := r.PathValue("teamId")
	teamID, err := strconv.ParseInt(teamStr, 10, 64)
	if err != nil || teamID < 1 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid team id"})
		return
	}
	ctx := r.Context()
	ok, err := s.store.TeamExists(ctx, teamID)
	if err != nil {
		s.log.Error("team exists", slog.Any("err", err))
		s.writeJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{"error": "internal server error"},
		)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
		return
	}
	q := r.URL.Query()
	dr, err := validate.ParseRequiredDateRange(q.Get("from"), q.Get("to"))
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	matches, err := s.store.ListMatchesForTeam(ctx, teamID, dr.FromInclusive, dr.ToExclusive)
	if err != nil {
		s.log.Error("list matches", slog.Any("err", err))
		s.writeJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{"error": "internal server error"},
		)
		return
	}
	if matches == nil {
		matches = []db.Match{}
	}
	s.writeJSON(w, http.StatusOK, matches)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(v); err != nil {
		s.log.Error("encode json", slog.Any("err", err))
	}
}
