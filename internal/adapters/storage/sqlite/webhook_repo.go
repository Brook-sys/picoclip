package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"picoclip/internal/core/domain"
)

type WebhookRepository struct {
	db *sql.DB
}

func (r *WebhookRepository) CreateSubscription(ctx context.Context, subscription domain.WebhookSubscription) error {
	eventTypes, _ := json.Marshal(subscription.EventTypes)
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, `
		INSERT INTO webhook_subscriptions (id, name, url, secret, event_types, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, subscription.ID, subscription.Name, subscription.URL, subscription.Secret, string(eventTypes), subscription.Enabled, subscription.CreatedAt, subscription.UpdatedAt)
	return err
}

func (r *WebhookRepository) GetSubscription(ctx context.Context, id string) (domain.WebhookSubscription, error) {
	q := getQueryer(ctx, r.db)
	row := q.QueryRowContext(ctx, `
		SELECT id, name, url, secret, event_types, enabled, created_at, updated_at
		FROM webhook_subscriptions WHERE id = ?
	`, id)
	return scanWebhookSubscription(row)
}

func (r *WebhookRepository) ListSubscriptions(ctx context.Context) ([]domain.WebhookSubscription, error) {
	q := getQueryer(ctx, r.db)
	rows, err := q.QueryContext(ctx, `
		SELECT id, name, url, secret, event_types, enabled, created_at, updated_at
		FROM webhook_subscriptions ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.WebhookSubscription, 0)
	for rows.Next() {
		item, err := scanWebhookSubscription(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *WebhookRepository) UpdateSubscription(ctx context.Context, subscription domain.WebhookSubscription) error {
	eventTypes, _ := json.Marshal(subscription.EventTypes)
	q := getQueryer(ctx, r.db)
	res, err := q.ExecContext(ctx, `
		UPDATE webhook_subscriptions
		SET name = ?, url = ?, secret = ?, event_types = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, subscription.Name, subscription.URL, subscription.Secret, string(eventTypes), subscription.Enabled, subscription.UpdatedAt, subscription.ID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *WebhookRepository) CreateDelivery(ctx context.Context, delivery domain.WebhookDelivery) error {
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, `
		INSERT OR IGNORE INTO webhook_deliveries (id, subscription_id, event_id, event_type, url, status, attempts, request_body, response_status, response_body, last_error, next_attempt_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, delivery.ID, delivery.SubscriptionID, delivery.EventID, string(delivery.EventType), delivery.URL, string(delivery.Status), delivery.Attempts, delivery.RequestBody, delivery.ResponseStatus, delivery.ResponseBody, delivery.LastError, delivery.NextAttemptAt, delivery.CreatedAt, delivery.UpdatedAt)
	return err
}

func (r *WebhookRepository) ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.WebhookDelivery, error) {
	q := getQueryer(ctx, r.db)
	rows, err := q.QueryContext(ctx, `
		SELECT id, subscription_id, event_id, event_type, url, status, attempts, request_body, response_status, response_body, last_error, next_attempt_at, created_at, updated_at
		FROM webhook_deliveries
		WHERE status IN ('pending', 'failed') AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		ORDER BY created_at ASC LIMIT ?
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.WebhookDelivery, 0)
	for rows.Next() {
		item, err := scanWebhookDelivery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *WebhookRepository) UpdateDelivery(ctx context.Context, delivery domain.WebhookDelivery) error {
	q := getQueryer(ctx, r.db)
	res, err := q.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = ?, attempts = ?, response_status = ?, response_body = ?, last_error = ?, next_attempt_at = ?, updated_at = ?
		WHERE id = ?
	`, string(delivery.Status), delivery.Attempts, delivery.ResponseStatus, delivery.ResponseBody, delivery.LastError, delivery.NextAttemptAt, delivery.UpdatedAt, delivery.ID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *WebhookRepository) ListDeliveries(ctx context.Context, subscriptionID string, limit int) ([]domain.WebhookDelivery, error) {
	q := getQueryer(ctx, r.db)
	query := `
		SELECT id, subscription_id, event_id, event_type, url, status, attempts, request_body, response_status, response_body, last_error, next_attempt_at, created_at, updated_at
		FROM webhook_deliveries WHERE 1=1
	`
	args := []any{}
	if subscriptionID != "" {
		query += " AND subscription_id = ?"
		args = append(args, subscriptionID)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.WebhookDelivery, 0)
	for rows.Next() {
		item, err := scanWebhookDelivery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanWebhookSubscription(row scanner) (domain.WebhookSubscription, error) {
	var subscription domain.WebhookSubscription
	var eventTypesJSON string
	err := row.Scan(&subscription.ID, &subscription.Name, &subscription.URL, &subscription.Secret, &eventTypesJSON, &subscription.Enabled, &subscription.CreatedAt, &subscription.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.WebhookSubscription{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.WebhookSubscription{}, err
	}
	_ = json.Unmarshal([]byte(eventTypesJSON), &subscription.EventTypes)
	return subscription, nil
}

func scanWebhookDelivery(row scanner) (domain.WebhookDelivery, error) {
	var delivery domain.WebhookDelivery
	var status, eventType string
	err := row.Scan(&delivery.ID, &delivery.SubscriptionID, &delivery.EventID, &eventType, &delivery.URL, &status, &delivery.Attempts, &delivery.RequestBody, &delivery.ResponseStatus, &delivery.ResponseBody, &delivery.LastError, &delivery.NextAttemptAt, &delivery.CreatedAt, &delivery.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.WebhookDelivery{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	delivery.EventType = domain.EventType(eventType)
	delivery.Status = domain.WebhookDeliveryStatus(status)
	return delivery, nil
}
