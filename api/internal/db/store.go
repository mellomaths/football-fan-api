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
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	ShortName     string  `json:"short_name,omitempty"`
	EspnSlug      string  `json:"espn_slug,omitempty"`
	SoccerwayID   string  `json:"soccerway_id,omitempty"`
	TicketSaleURL *string `json:"ticket_sale_url,omitempty"`
}

// TeamTicketSalePatch updates teams.ticket_sale_url. When TicketSaleURLSet is false, the column is left unchanged.
// When TicketSaleURLSet is true, TicketSaleURL nil clears the column.
type TeamTicketSalePatch struct {
	TicketSaleURLSet bool
	TicketSaleURL    *string
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

// TicketAnnouncement is one scraped article row for GET /teams/{id}/tickets/announcements.
type TicketAnnouncement struct {
	SaleScheduleText string `json:"sale_schedule_text"`
	PricesText       string `json:"prices_text"`
	ScrapedAt        string `json:"scraped_at"`
	Match            *Match `json:"match"`
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
		       COALESCE(t.soccerway_id, ''),
		       t.ticket_sale_url
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
		var ticketURL sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.ShortName, &t.EspnSlug, &t.SoccerwayID, &ticketURL); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		t.TicketSaleURL = nullStrPtr(ticketURL)
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
	var ticketURL sql.NullString
	err := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT t.id,
		       t.name,
		       COALESCE(t.short_name, ''),
		       COALESCE(t.espn_slug, ''),
		       COALESCE(t.soccerway_id, ''),
		       t.ticket_sale_url
		FROM %s.teams t
		WHERE t.id = $1
	`, AppSchema), teamID).Scan(&t.ID, &t.Name, &t.ShortName, &t.EspnSlug, &t.SoccerwayID, &ticketURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Team{}, ErrTeamNotFound
		}
		return Team{}, fmt.Errorf("get team: %w", err)
	}
	t.TicketSaleURL = nullStrPtr(ticketURL)
	return t, nil
}

// PatchTeamTicketSaleURL updates ticket_sale_url when patch.TicketSaleURLSet is true.
func (s *Store) PatchTeamTicketSaleURL(ctx context.Context, teamID int64, patch TeamTicketSalePatch) (Team, error) {
	if !patch.TicketSaleURLSet {
		return s.GetTeamByID(ctx, teamID)
	}
	var url any
	if patch.TicketSaleURL != nil {
		url = *patch.TicketSaleURL
	} else {
		url = nil
	}
	tag, err := s.pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.teams SET ticket_sale_url = $2 WHERE id = $1`,
		AppSchema,
	), teamID, url)
	if err != nil {
		return Team{}, fmt.Errorf("patch team ticket url: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Team{}, ErrTeamNotFound
	}
	return s.GetTeamByID(ctx, teamID)
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

// ListTicketAnnouncementsForTeam returns ticket_announcements for seller_team_id where scraped_at
// is in [fromInclusive, toInclusive] (UTC), newest first. Match is nil when match_id is unset.
func (s *Store) ListTicketAnnouncementsForTeam(
	ctx context.Context,
	sellerTeamID int64,
	fromInclusive, toInclusive time.Time,
) ([]TicketAnnouncement, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT ta.sale_schedule_text,
		       ta.prices_text,
		       ta.scraped_at,
		       m.id,
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
		FROM %s.ticket_announcements ta
		LEFT JOIN %s.matches m ON m.id = ta.match_id
		LEFT JOIN %s.teams ht ON ht.id = m.home_team_id
		LEFT JOIN %s.teams at ON at.id = m.away_team_id
		LEFT JOIN %s.competitions c ON c.id = m.competition_id
		WHERE ta.seller_team_id = $1
		  AND ta.scraped_at >= $2
		  AND ta.scraped_at <= $3
		ORDER BY ta.scraped_at DESC
	`, AppSchema, AppSchema, AppSchema, AppSchema, AppSchema),
		sellerTeamID, fromInclusive.UTC(), toInclusive.UTC())
	if err != nil {
		return nil, fmt.Errorf("list ticket announcements: %w", err)
	}
	defer rows.Close()

	out := make([]TicketAnnouncement, 0)
	for rows.Next() {
		var ann TicketAnnouncement
		var scraped time.Time
		var mid sql.NullInt64
		var kickoff sql.NullTime
		var venue sql.NullString
		var hid, aid sql.NullInt64
		var hname, aname sql.NullString
		var htShort, htEspn, htSw sql.NullString
		var atShort, atEspn, atSw sql.NullString
		var cid sql.NullInt64
		var cname, ccode sql.NullString
		if err := rows.Scan(
			&ann.SaleScheduleText,
			&ann.PricesText,
			&scraped,
			&mid,
			&kickoff,
			&venue,
			&hid,
			&hname,
			&htShort,
			&htEspn,
			&htSw,
			&aid,
			&aname,
			&atShort,
			&atEspn,
			&atSw,
			&cid,
			&cname,
			&ccode,
		); err != nil {
			return nil, fmt.Errorf("scan ticket announcement: %w", err)
		}
		ann.ScrapedAt = scraped.UTC().Format(time.RFC3339)
		if !mid.Valid || !kickoff.Valid || !hid.Valid || !aid.Valid || !cid.Valid {
			ann.Match = nil
			out = append(out, ann)
			continue
		}
		m := Match{
			ID: mid.Int64,
			Home: TeamMatchSide{
				ID:   hid.Int64,
				Name: hname.String,
			},
			Away: TeamMatchSide{
				ID:   aid.Int64,
				Name: aname.String,
			},
			Competition: CompetitionRef{
				ID:   cid.Int64,
				Name: cname.String,
				Code: ccode.String,
			},
		}
		m.KickoffUTC = kickoff.Time.UTC().Format(time.RFC3339)
		if venue.Valid && venue.String != "" {
			m.Location = &LocationRef{Name: venue.String}
		}
		m.Home.ShortName = nullStrPtr(htShort)
		m.Home.EspnSlug = nullStrPtr(htEspn)
		m.Home.SoccerwayID = nullStrPtr(htSw)
		m.Away.ShortName = nullStrPtr(atShort)
		m.Away.EspnSlug = nullStrPtr(atEspn)
		m.Away.SoccerwayID = nullStrPtr(atSw)
		ann.Match = &m
		out = append(out, ann)
	}
	return out, rows.Err()
}
