-- Ticket sale URL per team (listing or portal; interpreted by scraper adapter).
ALTER TABLE footballfan.teams
    ADD COLUMN ticket_sale_url VARCHAR(1024);

-- Per-tier sale windows scraped from official announcements (e.g. Flamengo notícias).
CREATE TABLE footballfan.ticket_sale_windows (
    id               BIGSERIAL PRIMARY KEY,
    seller_team_id   BIGINT NOT NULL REFERENCES footballfan.teams (id) ON DELETE CASCADE,
    match_id         BIGINT REFERENCES footballfan.matches (id) ON DELETE SET NULL,
    opens_at_utc     TIMESTAMPTZ NOT NULL,
    tier_label       TEXT NOT NULL,
    sale_kind        VARCHAR(64),
    source           VARCHAR(32) NOT NULL,
    source_url       TEXT NOT NULL,
    raw_excerpt      TEXT,
    scraped_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_ticket_sale_window UNIQUE (seller_team_id, source, source_url, opens_at_utc, tier_label)
);

CREATE INDEX idx_ticket_sale_windows_team_opens ON footballfan.ticket_sale_windows (seller_team_id, opens_at_utc);
CREATE INDEX idx_ticket_sale_windows_match ON footballfan.ticket_sale_windows (match_id);
CREATE INDEX idx_ticket_sale_windows_scraped ON footballfan.ticket_sale_windows (scraped_at);

-- Ticket price lines per sector/tier from the same source article.
CREATE TABLE footballfan.ticket_price_quotes (
    id               BIGSERIAL PRIMARY KEY,
    seller_team_id   BIGINT NOT NULL REFERENCES footballfan.teams (id) ON DELETE CASCADE,
    match_id         BIGINT REFERENCES footballfan.matches (id) ON DELETE SET NULL,
    source_url       TEXT NOT NULL,
    sector           TEXT NOT NULL,
    tier_name        TEXT NOT NULL,
    price_full_brl   NUMERIC(12, 2) NOT NULL,
    price_half_brl   NUMERIC(12, 2),
    source           VARCHAR(32) NOT NULL,
    scraped_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_ticket_price_quote UNIQUE (seller_team_id, source_url, sector, tier_name)
);

CREATE INDEX idx_ticket_price_quotes_team ON footballfan.ticket_price_quotes (seller_team_id);
CREATE INDEX idx_ticket_price_quotes_match ON footballfan.ticket_price_quotes (match_id);
