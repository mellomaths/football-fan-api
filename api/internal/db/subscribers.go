package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrSubscriptionDuplicate is returned when the subscriber is already subscribed to the team.
var ErrSubscriptionDuplicate = errors.New("db: subscription already exists")

// Subscriber is a registered integration user (POST /users response body).
type Subscriber struct {
	ID             int64           `json:"id"`
	ExternalKey    string          `json:"external_key"`
	DeliveryTarget string          `json:"delivery_target"`
	DisplayName    *string         `json:"display_name,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

// SubscriptionRow is one team subscription for dispatch (GET /users/subscriptions).
type SubscriptionRow struct {
	SubscriberID   int64  `json:"subscriber_id"`
	DeliveryTarget string `json:"delivery_target"`
	TeamID         int64  `json:"team_id"`
}

// UpsertSubscriber inserts or updates by external_key.
func (s *Store) UpsertSubscriber(
	ctx context.Context,
	externalKey, deliveryTarget string,
	displayName *string,
	metadata json.RawMessage,
) (Subscriber, error) {
	if externalKey == "" || deliveryTarget == "" {
		return Subscriber{}, fmt.Errorf("upsert subscriber: external_key and delivery_target are required")
	}
	var meta any
	if len(metadata) > 0 {
		meta = metadata
	} else {
		meta = nil
	}
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.subscribers (external_key, delivery_target, display_name, metadata)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (external_key) DO UPDATE SET
			delivery_target = EXCLUDED.delivery_target,
			display_name = EXCLUDED.display_name,
			metadata = EXCLUDED.metadata,
			updated_at = now()
		RETURNING id, external_key, delivery_target, display_name, metadata
	`, AppSchema), externalKey, deliveryTarget, displayName, meta)

	var out Subscriber
	var dispName *string
	var metaBytes []byte
	if err := row.Scan(&out.ID, &out.ExternalKey, &out.DeliveryTarget, &dispName, &metaBytes); err != nil {
		return Subscriber{}, fmt.Errorf("upsert subscriber: %w", err)
	}
	out.DisplayName = dispName
	if len(metaBytes) > 0 {
		out.Metadata = metaBytes
	}
	return out, nil
}

// AddSubscriberTeamSubscription links a subscriber to a team; ErrSubscriptionDuplicate if already linked.
func (s *Store) AddSubscriberTeamSubscription(ctx context.Context, subscriberID, teamID int64) error {
	if subscriberID < 1 || teamID < 1 {
		return fmt.Errorf("add subscription: invalid ids")
	}
	tag, err := s.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.subscriber_team_subscriptions (subscriber_id, team_id)
		VALUES ($1, $2)
	`, AppSchema), subscriberID, teamID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrSubscriptionDuplicate
		}
		return fmt.Errorf("add subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionDuplicate
	}
	return nil
}

// ListSubscriberSubscriptions returns all subscriber–team pairs for notification dispatch.
func (s *Store) ListSubscriberSubscriptions(ctx context.Context) ([]SubscriptionRow, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT s.id, s.delivery_target, st.team_id
		FROM %s.subscribers s
		JOIN %s.subscriber_team_subscriptions st ON st.subscriber_id = s.id
		ORDER BY s.id, st.team_id
	`, AppSchema, AppSchema))
	if err != nil {
		return nil, fmt.Errorf("list subscriber subscriptions: %w", err)
	}
	defer rows.Close()

	out := make([]SubscriptionRow, 0)
	for rows.Next() {
		var r SubscriptionRow
		if err := rows.Scan(&r.SubscriberID, &r.DeliveryTarget, &r.TeamID); err != nil {
			return nil, fmt.Errorf("scan subscription row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TryInsertNotificationReceipt records that a notification was sent; returns false if already recorded.
func (s *Store) TryInsertNotificationReceipt(ctx context.Context, subscriberID, announcementID int64) (inserted bool, err error) {
	if subscriberID < 1 || announcementID < 1 {
		return false, fmt.Errorf("notification receipt: invalid ids")
	}
	tag, err := s.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.notification_receipts (subscriber_id, announcement_id)
		VALUES ($1, $2)
		ON CONFLICT (subscriber_id, announcement_id) DO NOTHING
	`, AppSchema), subscriberID, announcementID)
	if err != nil {
		return false, fmt.Errorf("notification receipt: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ErrSubscriberNotFound is returned when no subscriber exists for the id.
var ErrSubscriberNotFound = errors.New("db: subscriber not found")

func (s *Store) GetSubscriberByID(ctx context.Context, id int64) (Subscriber, error) {
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, external_key, delivery_target, display_name, metadata
		FROM %s.subscribers WHERE id = $1
	`, AppSchema), id)
	var out Subscriber
	var dispName *string
	var metaBytes []byte
	err := row.Scan(&out.ID, &out.ExternalKey, &out.DeliveryTarget, &dispName, &metaBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Subscriber{}, ErrSubscriberNotFound
		}
		return Subscriber{}, fmt.Errorf("get subscriber: %w", err)
	}
	out.DisplayName = dispName
	if len(metaBytes) > 0 {
		out.Metadata = metaBytes
	}
	return out, nil
}
