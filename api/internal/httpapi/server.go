// Package httpapi implements the HTTP API for teams and matches.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/mellomaths/football-fan-api/api/internal/db"
	"github.com/mellomaths/football-fan-api/api/internal/validate"
)

// Store is the read model used by HTTP handlers (implemented by *db.Store).
type Store interface {
	ListTeams(ctx context.Context) ([]db.Team, error)
	GetTeamByID(ctx context.Context, teamID int64) (db.Team, error)
	TeamExists(ctx context.Context, teamID int64) (bool, error)
	ListMatchesForTeam(ctx context.Context, teamID int64, fromInclusive, toExclusive time.Time) ([]db.Match, error)
}

// Server wires HTTP handlers to the data store.
type Server struct {
	log   *slog.Logger
	store Store
}

// NewServer returns an API server.
func NewServer(log *slog.Logger, store Store) *Server {
	return &Server{log: log, store: store}
}

// Handler returns the root mux with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /teams", s.handleTeams)
	mux.HandleFunc("GET /teams/{teamId}/matches", s.handleTeamMatches)
	mux.HandleFunc("GET /teams/{teamId}", s.handleTeamByID)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
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
	teams, err := s.store.ListTeams(ctx)
	if err != nil {
		s.log.Error("list teams", slog.Any("err", err))
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
