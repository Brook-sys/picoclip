package memory

import (
	"context"
	"sort"

	"picoclip/internal/core/domain"
)

func (r messageRepository) Create(ctx context.Context, message domain.Message) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.messages[message.ID] = message
	return nil
}

func (r messageRepository) ListByTask(ctx context.Context, taskID string) ([]domain.Message, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	messages := make([]domain.Message, 0)
	for _, message := range r.storage.messages {
		if message.TaskID == taskID {
			messages = append(messages, message)
		}
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].CreatedAt.Before(messages[j].CreatedAt) })
	return messages, nil
}
