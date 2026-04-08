-- Subscribers and team subscriptions (vendor-neutral; no messaging-specific columns).
CREATE TABLE footballfan.subscribers (
    id               BIGSERIAL PRIMARY KEY,
    external_key     TEXT NOT NULL UNIQUE,
    delivery_target  TEXT NOT NULL,
    display_name     TEXT,
    metadata         JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE footballfan.subscriber_team_subscriptions (
    subscriber_id BIGINT NOT NULL REFERENCES footballfan.subscribers (id) ON DELETE CASCADE,
    team_id       BIGINT NOT NULL REFERENCES footballfan.teams (id) ON DELETE CASCADE,
    PRIMARY KEY (subscriber_id, team_id)
);

CREATE INDEX idx_subscriber_team_subscriptions_team
    ON footballfan.subscriber_team_subscriptions (team_id);

CREATE TABLE footballfan.notification_receipts (
    subscriber_id    BIGINT NOT NULL REFERENCES footballfan.subscribers (id) ON DELETE CASCADE,
    announcement_id  BIGINT NOT NULL REFERENCES footballfan.ticket_announcements (id) ON DELETE CASCADE,
    sent_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (subscriber_id, announcement_id)
);
