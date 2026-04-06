package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mellomaths/football-fan-api/api/internal/db"
)

type stubStore struct {
	teams    []db.Team
	teamByID map[int64]db.Team
	exists   map[int64]bool
	matches  []db.Match
	err      error
}

func (s *stubStore) ListTeams(_ context.Context) ([]db.Team, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.teams, nil
}

func (s *stubStore) GetTeamByID(_ context.Context, teamID int64) (db.Team, error) {
	if s.err != nil {
		return db.Team{}, s.err
	}
	if d, ok := s.teamByID[teamID]; ok {
		return d, nil
	}
	return db.Team{}, db.ErrTeamNotFound
}

func (s *stubStore) TeamExists(_ context.Context, teamID int64) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.exists[teamID], nil
}

func (s *stubStore) ListMatchesForTeam(
	_ context.Context,
	_ int64,
	_, _ time.Time,
) ([]db.Match, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.matches, nil
}

func TestHandleTeamMatchesValidation(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := &stubStore{exists: map[int64]bool{1: true}}
	srv := NewServer(log, st)

	req := httptest.NewRequest(http.MethodGet, "/teams/1/matches", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestHandleTeamMatchesOK(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := &stubStore{
		exists: map[int64]bool{1: true},
		matches: []db.Match{
			{
				ID:         10,
				KickoffUTC: "2026-04-15T20:00:00Z",
				Location:   &db.LocationRef{Name: "Estadio X"},
				Home:       db.TeamMatchSide{ID: 1, Name: "A"},
				Away:       db.TeamMatchSide{ID: 2, Name: "B"},
				Competition: db.CompetitionRef{
					ID:   1,
					Name: "Y",
					Code: "X",
				},
			},
		},
	}
	srv := NewServer(log, st)

	req := httptest.NewRequest(
		http.MethodGet,
		"/teams/1/matches?from=2026-04-01&to=2026-04-30",
		nil,
	)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var out []db.Match
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != 10 {
		t.Fatalf("unexpected body: %+v", out)
	}
	if out[0].KickoffUTC != "2026-04-15T20:00:00Z" || out[0].Home.Name != "A" || out[0].Competition.Code != "X" {
		t.Fatalf("unexpected match shape: %+v", out[0])
	}
}

func TestHandleTeamByIDOK(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := &stubStore{
		teamByID: map[int64]db.Team{
			7: {
				ID:          7,
				Name:        "Flamengo",
				ShortName:   "FLA",
				EspnSlug:    "flamengo",
				SoccerwayID: "flamengo",
			},
		},
	}
	srv := NewServer(log, st)

	req := httptest.NewRequest(http.MethodGet, "/teams/7", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var out db.Team
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ID != 7 || out.Name != "Flamengo" || out.ShortName != "FLA" {
		t.Fatalf("unexpected body: %+v", out)
	}
}

func TestHandleTeamByIDNotFound(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := &stubStore{teamByID: map[int64]db.Team{}}
	srv := NewServer(log, st)

	req := httptest.NewRequest(http.MethodGet, "/teams/99", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}
