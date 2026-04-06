// Package db defines persistence types and read queries against PostgreSQL.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrTeamNotFound is returned by GetTeamByID when no row exists for the id.
var ErrTeamNotFound = errors.New("db: team not found")

// Team is the JSON body for GET /teams and GET /teams/{teamId}.
type Team struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ShortName   string `json:"short_name,omitempty"`
	EspnSlug    string `json:"espn_slug,omitempty"`
	SoccerwayID string `json:"soccerway_id,omitempty"`
}

// TeamMatchSide is a club in a match response (GET /teams/{id}/matches).
type TeamMatchSide struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	ShortName   *string `json:"short_name,omitempty"`
	EspnSlug    *string `json:"espn_slug,omitempty"`
	SoccerwayID *string `json:"soccerway_id,omitempty"`
}

// CompetitionRef identifies the competition for a match.
type CompetitionRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// LocationRef is the venue for a match (GET /teams/{id}/matches).
type LocationRef struct {
	Name string `json:"name"`
}

// Match is a scheduled match involving the requested team.
type Match struct {
	ID          int64          `json:"id"`
	KickoffUTC  string         `json:"kickoff_utc"`
	Location    *LocationRef   `json:"location,omitempty"`
	Home        TeamMatchSide  `json:"home"`
	Away        TeamMatchSide  `json:"away"`
	Competition CompetitionRef `json:"competition"`
}

// Store performs read queries against PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps a pgx pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ListTeams returns all teams ordered by primary competition code then name.
// Uses primary membership when set; otherwise picks one linked competition (is_primary DESC).
func (s *Store) ListTeams(ctx context.Context) ([]Team, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT ON (t.id)
		       t.id,
		       t.name,
		       COALESCE(t.short_name, ''),
		       COALESCE(t.espn_slug, ''),
		       COALESCE(t.soccerway_id, '')
		FROM %s.teams t
		JOIN %s.team_competitions tc ON tc.team_id = t.id
		JOIN %s.competitions c ON c.id = tc.competition_id
		ORDER BY t.id, tc.is_primary DESC, c.code
	`, AppSchema, AppSchema, AppSchema))
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	out := make([]Team, 0)
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.ShortName, &t.EspnSlug, &t.SoccerwayID); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TeamExists reports whether a team id exists.
func (s *Store) TeamExists(ctx context.Context, teamID int64) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS(SELECT 1 FROM %s.teams WHERE id = $1)`,
		AppSchema,
	), teamID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("team exists: %w", err)
	}
	return exists, nil
}

// GetTeamByID returns one team by primary key, or ErrTeamNotFound.
func (s *Store) GetTeamByID(ctx context.Context, teamID int64) (Team, error) {
	var t Team
	err := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT t.id,
		       t.name,
		       COALESCE(t.short_name, ''),
		       COALESCE(t.espn_slug, ''),
		       COALESCE(t.soccerway_id, '')
		FROM %s.teams t
		WHERE t.id = $1
	`, AppSchema), teamID).Scan(&t.ID, &t.Name, &t.ShortName, &t.EspnSlug, &t.SoccerwayID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Team{}, ErrTeamNotFound
		}
		return Team{}, fmt.Errorf("get team: %w", err)
	}
	return t, nil
}

func nullStrPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

// ListMatchesForTeam returns matches where kickoff_utc is in [fromInclusive, toExclusive).
func (s *Store) ListMatchesForTeam(ctx context.Context, teamID int64, fromInclusive, toExclusive time.Time) ([]Match, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT m.id,
		       m.kickoff_utc,
		       COALESCE(m.venue, ''),
		       ht.id,
		       ht.name,
		       ht.short_name,
		       ht.espn_slug,
		       ht.soccerway_id,
		       at.id,
		       at.name,
		       at.short_name,
		       at.espn_slug,
		       at.soccerway_id,
		       c.id,
		       c.name,
		       c.code
		FROM %s.matches m
		JOIN %s.teams ht ON ht.id = m.home_team_id
		JOIN %s.teams at ON at.id = m.away_team_id
		JOIN %s.competitions c ON c.id = m.competition_id
		WHERE (m.home_team_id = $1 OR m.away_team_id = $1)
		  AND m.kickoff_utc >= $2
		  AND m.kickoff_utc < $3
		ORDER BY m.kickoff_utc ASC
	`, AppSchema, AppSchema, AppSchema, AppSchema), teamID, fromInclusive, toExclusive)
	if err != nil {
		return nil, fmt.Errorf("list matches: %w", err)
	}
	defer rows.Close()

	out := make([]Match, 0)
	for rows.Next() {
		var m Match
		var kickoff time.Time
		var venue string
		var htShort, htEspn, htSw sql.NullString
		var atShort, atEspn, atSw sql.NullString
		if err := rows.Scan(
			&m.ID,
			&kickoff,
			&venue,
			&m.Home.ID,
			&m.Home.Name,
			&htShort,
			&htEspn,
			&htSw,
			&m.Away.ID,
			&m.Away.Name,
			&atShort,
			&atEspn,
			&atSw,
			&m.Competition.ID,
			&m.Competition.Name,
			&m.Competition.Code,
		); err != nil {
			return nil, fmt.Errorf("scan match: %w", err)
		}
		m.KickoffUTC = kickoff.UTC().Format(time.RFC3339)
		if venue != "" {
			m.Location = &LocationRef{Name: venue}
		}
		m.Home.ShortName = nullStrPtr(htShort)
		m.Home.EspnSlug = nullStrPtr(htEspn)
		m.Home.SoccerwayID = nullStrPtr(htSw)
		m.Away.ShortName = nullStrPtr(atShort)
		m.Away.EspnSlug = nullStrPtr(atEspn)
		m.Away.SoccerwayID = nullStrPtr(atSw)
		out = append(out, m)
	}
	return out, rows.Err()
}
