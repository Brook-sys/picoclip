package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

const maxWebhookAttempts = 10

type WebhookService struct {
	storage ports.Storage
	clock   ports.Clock
	idGen   ports.IDGenerator
}

func NewWebhookService(storage ports.Storage, clock ports.Clock, idGen ports.IDGenerator) *WebhookService {
	return &WebhookService{storage: storage, clock: clock, idGen: idGen}
}

func (s *WebhookService) CreateSubscription(ctx context.Context, subscription domain.WebhookSubscription) (domain.WebhookSubscription, error) {
	if strings.TrimSpace(subscription.URL) == "" {
		return domain.WebhookSubscription{}, fmt.Errorf("%w: url is required", domain.ErrInvalidInput)
	}
	if subscription.ID == "" {
		subscription.ID = s.idGen.NewID("wh")
	}
	if strings.TrimSpace(subscription.Name) == "" {
		subscription.Name = subscription.URL
	}
	now := s.clock.Now()
	if subscription.CreatedAt.IsZero() {
		subscription.CreatedAt = now
	}
	subscription.UpdatedAt = now
	return subscription, s.storage.Webhooks().CreateSubscription(ctx, subscription)
}

func WebhookEventMatches(subscription domain.WebhookSubscription, event domain.Event) bool {
	if !subscription.Enabled {
		return false
	}
	if len(subscription.EventTypes) == 0 {
		return true
	}
	for _, eventType := range subscription.EventTypes {
		if eventType == event.Type {
			return true
		}
	}
	return false
}

func EnqueueWebhookDeliveries(ctx context.Context, storage ports.Storage, event domain.Event, now time.Time) error {
	subscriptions, err := storage.Webhooks().ListSubscriptions(ctx)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(event)
	for _, subscription := range subscriptions {
		if !WebhookEventMatches(subscription, event) {
			continue
		}
		delivery := domain.WebhookDelivery{
			ID:             "whd_" + event.ID + "_" + subscription.ID,
			SubscriptionID: subscription.ID,
			EventID:        event.ID,
			EventType:      event.Type,
			URL:            subscription.URL,
			Status:         domain.WebhookDeliveryPending,
			RequestBody:    string(payload),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := storage.Webhooks().CreateDelivery(ctx, delivery); err != nil {
			return err
		}
	}
	return nil
}

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type WebhookDeliveryWorker struct {
	storage ports.Storage
	clock   ports.Clock
	client  HTTPDoer
}

func NewWebhookDeliveryWorker(storage ports.Storage, clock ports.Clock, client HTTPDoer) *WebhookDeliveryWorker {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &WebhookDeliveryWorker{storage: storage, clock: clock, client: client}
}

func (w *WebhookDeliveryWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ProcessDue(ctx)
		}
	}
}

func (w *WebhookDeliveryWorker) ProcessDue(ctx context.Context) {
	deliveries, err := w.storage.Webhooks().ListDueDeliveries(ctx, w.clock.Now(), 25)
	if err != nil {
		return
	}
	for _, delivery := range deliveries {
		w.deliver(ctx, delivery)
	}
}

func (w *WebhookDeliveryWorker) deliver(ctx context.Context, delivery domain.WebhookDelivery) {
	subscription, err := w.storage.Webhooks().GetSubscription(ctx, delivery.SubscriptionID)
	if err != nil || !subscription.Enabled {
		return
	}

	body := []byte(delivery.RequestBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.URL, bytes.NewReader(body))
	if err != nil {
		w.markFailed(ctx, delivery, 0, "", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "PicoClip-Webhooks/1.0")
	req.Header.Set("X-PicoClip-Event", string(delivery.EventType))
	req.Header.Set("X-PicoClip-Delivery", delivery.ID)
	if subscription.Secret != "" {
		req.Header.Set("X-PicoClip-Signature", signWebhookPayload(subscription.Secret, body))
	}

	resp, err := w.client.Do(req)
	if err != nil {
		w.markFailed(ctx, delivery, 0, "", err.Error())
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		delivery.Status = domain.WebhookDeliveryDelivered
		delivery.Attempts++
		delivery.ResponseStatus = resp.StatusCode
		delivery.ResponseBody = string(respBody)
		delivery.LastError = ""
		delivery.NextAttemptAt = nil
		delivery.UpdatedAt = w.clock.Now()
		_ = w.storage.Webhooks().UpdateDelivery(ctx, delivery)
		return
	}
	w.markFailed(ctx, delivery, resp.StatusCode, string(respBody), fmt.Sprintf("unexpected status %d", resp.StatusCode))
}

func (w *WebhookDeliveryWorker) markFailed(ctx context.Context, delivery domain.WebhookDelivery, status int, responseBody string, message string) {
	delivery.Attempts++
	delivery.ResponseStatus = status
	delivery.ResponseBody = responseBody
	delivery.LastError = message
	now := w.clock.Now()
	delivery.UpdatedAt = now
	if delivery.Attempts >= maxWebhookAttempts {
		delivery.Status = domain.WebhookDeliveryDead
		delivery.NextAttemptAt = nil
	} else {
		delivery.Status = domain.WebhookDeliveryFailed
		next := now.Add(time.Duration(delivery.Attempts*delivery.Attempts) * time.Second)
		delivery.NextAttemptAt = &next
	}
	_ = w.storage.Webhooks().UpdateDelivery(ctx, delivery)
}

func signWebhookPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
