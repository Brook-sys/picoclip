package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/services"
)

type webhookCreateRequest struct {
	Name       string             `json:"name"`
	URL        string             `json:"url"`
	Secret     string             `json:"secret"`
	EventTypes []domain.EventType `json:"event_types"`
	Enabled    *bool              `json:"enabled"`
}

type webhookResponse struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	URL        string             `json:"url"`
	EventTypes []domain.EventType `json:"event_types"`
	Enabled    bool               `json:"enabled"`
	SecretSet  bool               `json:"secret_set"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

func newWebhookResponse(subscription domain.WebhookSubscription) webhookResponse {
	return webhookResponse{
		ID:         subscription.ID,
		Name:       subscription.Name,
		URL:        subscription.URL,
		EventTypes: subscription.EventTypes,
		Enabled:    subscription.Enabled,
		SecretSet:  subscription.Secret != "",
		CreatedAt:  subscription.CreatedAt,
		UpdatedAt:  subscription.UpdatedAt,
	}
}

func (s *Server) handleAPIV1Webhooks(w http.ResponseWriter, r *http.Request) {
	items, err := s.storage.Webhooks().ListSubscriptions(r.Context())
	if err != nil {
		s.apiError(w, err)
		return
	}
	responses := make([]webhookResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, newWebhookResponse(item))
	}
	s.apiData(w, responses)
}

func (s *Server) handleAPIV1CreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req webhookCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err))
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		s.apiError(w, fmt.Errorf("%w: url is required", domain.ErrInvalidInput))
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	svc := services.NewWebhookService(s.storage, services.SystemClock{}, &services.TimeIDGenerator{})
	subscription, err := svc.CreateSubscription(r.Context(), domain.WebhookSubscription{
		Name:       strings.TrimSpace(req.Name),
		URL:        strings.TrimSpace(req.URL),
		Secret:     req.Secret,
		EventTypes: req.EventTypes,
		Enabled:    enabled,
	})
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, newWebhookResponse(subscription))
}

func (s *Server) handleAPIV1WebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := s.storage.Webhooks().ListDeliveries(r.Context(), id, 100)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, items)
}
