package domain

import "time"

type WebhookDeliveryStatus string

const (
	WebhookDeliveryPending   WebhookDeliveryStatus = "pending"
	WebhookDeliveryDelivered WebhookDeliveryStatus = "delivered"
	WebhookDeliveryFailed    WebhookDeliveryStatus = "failed"
	WebhookDeliveryDead      WebhookDeliveryStatus = "dead"
)

type WebhookSubscription struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	URL        string      `json:"url"`
	Secret     string      `json:"secret,omitempty"`
	EventTypes []EventType `json:"event_types"`
	Enabled    bool        `json:"enabled"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

type WebhookDelivery struct {
	ID             string                `json:"id"`
	SubscriptionID string                `json:"subscription_id"`
	EventID        string                `json:"event_id"`
	EventType      EventType             `json:"event_type"`
	URL            string                `json:"url"`
	Status         WebhookDeliveryStatus `json:"status"`
	Attempts       int                   `json:"attempts"`
	RequestBody    string                `json:"request_body"`
	ResponseStatus int                   `json:"response_status"`
	ResponseBody   string                `json:"response_body"`
	LastError      string                `json:"last_error"`
	NextAttemptAt  *time.Time            `json:"next_attempt_at,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}
