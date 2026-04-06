-- Application objects live in schema footballfan.
-- Competitions: national leagues, cups (Libertadores, Copa do Brasil, estaduais, etc.)
CREATE SCHEMA IF NOT EXISTS footballfan;

CREATE TABLE footballfan.competitions (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(64) NOT NULL UNIQUE,
    name        VARCHAR(255) NOT NULL
);

-- One row per club; competition membership is modeled in team_competitions.
CREATE TABLE footballfan.teams (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL UNIQUE,
    short_name  VARCHAR(64),
    espn_slug   VARCHAR(128),
    soccerway_id VARCHAR(128)
);

-- Many-to-many: which competitions a team enters (season optional; exactly one is_primary per team).
CREATE TABLE footballfan.team_competitions (
    team_id         BIGINT NOT NULL REFERENCES footballfan.teams (id) ON DELETE CASCADE,
    competition_id  BIGINT NOT NULL REFERENCES footballfan.competitions (id) ON DELETE RESTRICT,
    season          VARCHAR(32),
    is_primary      BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (team_id, competition_id)
);

CREATE UNIQUE INDEX uq_team_competitions_one_primary
    ON footballfan.team_competitions (team_id)
    WHERE is_primary;

CREATE INDEX idx_team_competitions_competition ON footballfan.team_competitions (competition_id);

CREATE TABLE footballfan.matches (
    id                  BIGSERIAL PRIMARY KEY,
    competition_id           BIGINT NOT NULL REFERENCES footballfan.competitions (id) ON DELETE RESTRICT,
    home_team_id        BIGINT NOT NULL REFERENCES footballfan.teams (id) ON DELETE RESTRICT,
    away_team_id        BIGINT NOT NULL REFERENCES footballfan.teams (id) ON DELETE RESTRICT,
    kickoff_utc         TIMESTAMPTZ NOT NULL,
    venue               VARCHAR(512),
    source              VARCHAR(32) NOT NULL,
    external_match_id   VARCHAR(256) NOT NULL,
    scraped_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_match_source_external UNIQUE (source, external_match_id),
    CONSTRAINT chk_home_away CHECK (home_team_id <> away_team_id)
);

CREATE INDEX idx_matches_kickoff ON footballfan.matches (kickoff_utc);
CREATE INDEX idx_matches_home ON footballfan.matches (home_team_id);
CREATE INDEX idx_matches_away ON footballfan.matches (away_team_id);
CREATE INDEX idx_matches_competition ON footballfan.matches (competition_id);
