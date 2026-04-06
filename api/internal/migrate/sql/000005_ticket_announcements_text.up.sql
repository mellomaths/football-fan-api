-- Replace normalized ticket rows with one row per article: full section blobs.
DROP TABLE IF EXISTS footballfan.ticket_price_quotes;
DROP TABLE IF EXISTS footballfan.ticket_sale_windows;

CREATE TABLE footballfan.ticket_announcements (
    id                  BIGSERIAL PRIMARY KEY,
    seller_team_id      BIGINT NOT NULL REFERENCES footballfan.teams (id) ON DELETE CASCADE,
    match_id            BIGINT REFERENCES footballfan.matches (id) ON DELETE SET NULL,
    source              VARCHAR(32) NOT NULL,
    source_url          TEXT NOT NULL,
    sale_schedule_text  TEXT NOT NULL DEFAULT '',
    prices_text         TEXT NOT NULL DEFAULT '',
    scraped_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_ticket_announcement UNIQUE (seller_team_id, source, source_url)
);

CREATE INDEX idx_ticket_announcements_team ON footballfan.ticket_announcements (seller_team_id);
CREATE INDEX idx_ticket_announcements_scraped ON footballfan.ticket_announcements (scraped_at);
CREATE INDEX idx_ticket_announcements_match ON footballfan.ticket_announcements (match_id);
