package memory

import (
	"context"
	"sort"
	"time"

	"picoclip/internal/core/domain"
)

func (r webhookRepository) CreateSubscription(ctx context.Context, subscription domain.WebhookSubscription) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.webhooks[subscription.ID] = subscription
	return nil
}

func (r webhookRepository) GetSubscription(ctx context.Context, id string) (domain.WebhookSubscription, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	subscription, ok := r.storage.webhooks[id]
	if !ok {
		return domain.WebhookSubscription{}, domain.ErrNotFound
	}
	return subscription, nil
}

func (r webhookRepository) ListSubscriptions(ctx context.Context) ([]domain.WebhookSubscription, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	items := make([]domain.WebhookSubscription, 0, len(r.storage.webhooks))
	for _, subscription := range r.storage.webhooks {
		items = append(items, subscription)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (r webhookRepository) UpdateSubscription(ctx context.Context, subscription domain.WebhookSubscription) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.webhooks[subscription.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.webhooks[subscription.ID] = subscription
	return nil
}

func (r webhookRepository) DeleteSubscription(ctx context.Context, id string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.webhooks[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.storage.webhooks, id)
	for deliveryID, delivery := range r.storage.deliveries {
		if delivery.SubscriptionID == id {
			delete(r.storage.deliveries, deliveryID)
		}
	}
	return nil
}

func (r webhookRepository) CreateDelivery(ctx context.Context, delivery domain.WebhookDelivery) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.deliveries[delivery.ID] = delivery
	return nil
}

func (r webhookRepository) GetDelivery(ctx context.Context, id string) (domain.WebhookDelivery, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	delivery, ok := r.storage.deliveries[id]
	if !ok {
		return domain.WebhookDelivery{}, domain.ErrNotFound
	}
	return delivery, nil
}

func (r webhookRepository) ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.WebhookDelivery, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	items := make([]domain.WebhookDelivery, 0)
	for _, delivery := range r.storage.deliveries {
		if delivery.Status != domain.WebhookDeliveryPending && delivery.Status != domain.WebhookDeliveryFailed {
			continue
		}
		if delivery.NextAttemptAt != nil && delivery.NextAttemptAt.After(now) {
			continue
		}
		items = append(items, delivery)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r webhookRepository) UpdateDelivery(ctx context.Context, delivery domain.WebhookDelivery) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.deliveries[delivery.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.deliveries[delivery.ID] = delivery
	return nil
}

func (r webhookRepository) ListDeliveries(ctx context.Context, subscriptionID string, limit int) ([]domain.WebhookDelivery, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	items := make([]domain.WebhookDelivery, 0)
	for _, delivery := range r.storage.deliveries {
		if subscriptionID != "" && delivery.SubscriptionID != subscriptionID {
			continue
		}
		items = append(items, delivery)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
