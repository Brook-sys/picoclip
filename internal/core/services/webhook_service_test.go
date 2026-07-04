package services

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWebhookDeliveryWorkerSignsAndDelivers(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 16, 0, 0, 0, time.UTC)}
	sub := domain.WebhookSubscription{ID: "wh_1", Name: "test", URL: "https://example.test/hook", Secret: "secret", EventTypes: []domain.EventType{domain.EventTaskCreated}, Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Webhooks().CreateSubscription(context.Background(), sub); err != nil {
		t.Fatal(err)
	}
	event := domain.Event{ID: "evt_1", Type: domain.EventTaskCreated, TaskID: "task_1", Message: "created", CreatedAt: clock.t}
	if err := EnqueueWebhookDeliveries(context.Background(), st, event, clock.t); err != nil {
		t.Fatal(err)
	}

	var gotSignature string
	worker := NewWebhookDeliveryWorker(st, clock, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotSignature = req.Header.Get("X-PicoClip-Signature")
		if req.Header.Get("X-PicoClip-Event") != string(domain.EventTaskCreated) {
			t.Fatalf("unexpected event header %q", req.Header.Get("X-PicoClip-Event"))
		}
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), "evt_1") {
			t.Fatalf("unexpected body %s", string(body))
		}
		return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}))
	worker.ProcessDue(context.Background())
	if gotSignature == "" || !strings.HasPrefix(gotSignature, "sha256=") {
		t.Fatalf("missing signature %q", gotSignature)
	}
	deliveries, err := st.Webhooks().ListDeliveries(context.Background(), sub.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].Status != domain.WebhookDeliveryDelivered || deliveries[0].Attempts != 1 {
		t.Fatalf("unexpected deliveries %#v", deliveries)
	}
}

func TestWebhookDeliveryWorkerRetriesFailures(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 17, 0, 0, 0, time.UTC)}
	sub := domain.WebhookSubscription{ID: "wh_2", Name: "test", URL: "https://example.test/hook", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Webhooks().CreateSubscription(context.Background(), sub); err != nil {
		t.Fatal(err)
	}
	event := domain.Event{ID: "evt_2", Type: domain.EventRunFailed, CreatedAt: clock.t}
	if err := EnqueueWebhookDeliveries(context.Background(), st, event, clock.t); err != nil {
		t.Fatal(err)
	}
	worker := NewWebhookDeliveryWorker(st, clock, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("no"))}, nil
	}))
	worker.ProcessDue(context.Background())
	deliveries, err := st.Webhooks().ListDeliveries(context.Background(), sub.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].Status != domain.WebhookDeliveryFailed || deliveries[0].Attempts != 1 || deliveries[0].NextAttemptAt == nil {
		t.Fatalf("unexpected failed delivery %#v", deliveries)
	}
}
