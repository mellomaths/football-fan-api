package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mellomaths/football-fan-api/api/internal/db"
)

type stubStore struct {
	teams         []db.Team
	teamByID      map[int64]db.Team
	exists        map[int64]bool
	matches       []db.Match
	announcements []db.TicketAnnouncement
	subs          []db.SubscriptionRow
	err           error
}

func (s *stubStore) ListTeams(_ context.Context, nameQuery string) ([]db.Team, error) {
	if s.err != nil {
		return nil, s.err
	}
	if nameQuery == "" {
		return s.teams, nil
	}
	var out []db.Team
	nq := strings.ToLower(nameQuery)
	for _, t := range s.teams {
		if strings.Contains(strings.ToLower(t.Name), nq) ||
			strings.Contains(strings.ToLower(t.ShortName), nq) {
			out = append(out, t)
		}
	}
	return out, nil
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

func (s *stubStore) PatchTeamTicketSaleURL(_ context.Context, teamID int64, patch db.TeamTicketSalePatch) (db.Team, error) {
	if s.err != nil {
		return db.Team{}, s.err
	}
	d, ok := s.teamByID[teamID]
	if !ok {
		return db.Team{}, db.ErrTeamNotFound
	}
	if patch.TicketSaleURLSet {
		d.TicketSaleURL = patch.TicketSaleURL
	}
	s.teamByID[teamID] = d
	return d, nil
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

func (s *stubStore) ListTicketAnnouncementsForTeam(
	_ context.Context,
	_ int64,
	_, _ time.Time,
) ([]db.TicketAnnouncement, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.announcements, nil
}

func (s *stubStore) UpsertSubscriber(context.Context, string, string, *string, json.RawMessage) (db.Subscriber, error) {
	if s.err != nil {
		return db.Subscriber{}, s.err
	}
	return db.Subscriber{}, nil
}

func (s *stubStore) GetSubscriberByID(_ context.Context, _ int64) (db.Subscriber, error) {
	if s.err != nil {
		return db.Subscriber{}, s.err
	}
	return db.Subscriber{}, db.ErrSubscriberNotFound
}

func (s *stubStore) AddSubscriberTeamSubscription(context.Context, int64, int64) error {
	return s.err
}

func (s *stubStore) ListSubscriberSubscriptions(context.Context) ([]db.SubscriptionRow, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.subs, nil
}

func (s *stubStore) TryInsertNotificationReceipt(context.Context, int64, int64) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return true, nil
}

func TestHandleTeamsOK(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := &stubStore{
		teams: []db.Team{
			{ID: 1, Name: "CR Flamengo", ShortName: "CRF"},
			{ID: 2, Name: "Flamengo FC", ShortName: "FFC"},
		},
	}
	srv := NewServer(log, st, "")

	req := httptest.NewRequest(http.MethodGet, "/teams", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var out []db.Team
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len %d, want 2", len(out))
	}
}

func TestHandleTeamsNameFilter(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := &stubStore{
		teams: []db.Team{
			{ID: 1, Name: "CR Flamengo", ShortName: "CRF"},
			{ID: 2, Name: "Flamengo FC", ShortName: "FFC"},
			{ID: 3, Name: "Other Club", ShortName: "OC"},
		},
	}
	srv := NewServer(log, st, "")

	req := httptest.NewRequest(http.MethodGet, "/teams?name=Flamengo", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var out []db.Team
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len %d, want 2", len(out))
	}
}

func TestHandleTeamTicketAnnouncementsValidation(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := &stubStore{exists: map[int64]bool{1: true}}
	srv := NewServer(log, st, "")

	req := httptest.NewRequest(http.MethodGet, "/teams/1/tickets/announcements", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestHandleTeamTicketAnnouncementsOK(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := &db.Match{
		ID:         132,
		KickoffUTC: "2026-04-09T00:30:00Z",
		Location:   &db.LocationRef{Name: "Estadio Inca Garcilaso de la Vega"},
		Home:       db.TeamMatchSide{ID: 57, Name: "Cusco FC", ShortName: strPtr("Cusco FC"), EspnSlug: strPtr("cusco-fc")},
		Away:       db.TeamMatchSide{ID: 6, Name: "Flamengo", ShortName: strPtr("Flamengo"), EspnSlug: strPtr("flamengo")},
		Competition: db.CompetitionRef{
			ID:   3,
			Name: "Copa Libertadores",
			Code: "COPA_LIBERTADORES",
		},
	}
	st := &stubStore{
		exists: map[int64]bool{6: true},
		announcements: []db.TicketAnnouncement{
			{
				ID:               42,
				SaleScheduleText: "Data e hora das aberturas de vendas...",
				PricesText:       "Valores:",
				ScrapedAt:        "2026-04-09T00:30:00Z",
				Match:            m,
			},
		},
	}
	srv := NewServer(log, st, "")

	req := httptest.NewRequest(
		http.MethodGet,
		"/teams/6/tickets/announcements?from=2026-04-01T00:00:00Z&to=2026-04-30T23:59:59Z",
		nil,
	)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var out []db.TicketAnnouncement
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].SaleScheduleText != "Data e hora das aberturas de vendas..." {
		t.Fatalf("unexpected body: %+v", out)
	}
	if out[0].Match == nil || out[0].Match.ID != 132 || out[0].Match.Away.Name != "Flamengo" {
		t.Fatalf("unexpected match: %+v", out[0].Match)
	}
}

func strPtr(s string) *string {
	return &s
}

func TestHandleTeamMatchesValidation(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := &stubStore{exists: map[int64]bool{1: true}}
	srv := NewServer(log, st, "")

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
	srv := NewServer(log, st, "")

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
	srv := NewServer(log, st, "")

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
	srv := NewServer(log, st, "")

	req := httptest.NewRequest(http.MethodGet, "/teams/99", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}

func TestHandlePatchTeamTicketSaleURL(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	u := "https://www.flamengo.com.br/noticias/futebol"
	st := &stubStore{
		teamByID: map[int64]db.Team{
			1: {ID: 1, Name: "Flamengo", ShortName: "FLA"},
		},
	}
	srv := NewServer(log, st, "")

	req := httptest.NewRequest(
		http.MethodPatch,
		"/teams/1",
		strings.NewReader(`{"ticket_sale_url":"https://www.flamengo.com.br/noticias/futebol"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var out db.Team
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.TicketSaleURL == nil || *out.TicketSaleURL != u {
		t.Fatalf("unexpected ticket_sale_url: %+v", out.TicketSaleURL)
	}
}

func TestHandlePatchTeamClearTicketSaleURL(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	u := "https://example.com/x"
	st := &stubStore{
		teamByID: map[int64]db.Team{
			1: {ID: 1, Name: "A", TicketSaleURL: &u},
		},
	}
	srv := NewServer(log, st, "")

	req := httptest.NewRequest(http.MethodPatch, "/teams/1", strings.NewReader(`{"ticket_sale_url":null}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var out db.Team
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.TicketSaleURL != nil {
		t.Fatalf("want nil ticket_sale_url, got %+v", out.TicketSaleURL)
	}
}
