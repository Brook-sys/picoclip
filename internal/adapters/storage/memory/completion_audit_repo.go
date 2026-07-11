package memory

import (
	"context"
	"sort"

	"picoclip/internal/core/domain"
)

func (r completionAuditRepository) Create(_ context.Context, audit domain.CompletionAudit) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.completionAudits[audit.ID] = audit
	return nil
}

func (r completionAuditRepository) Update(_ context.Context, audit domain.CompletionAudit) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.completionAudits[audit.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.completionAudits[audit.ID] = audit
	return nil
}

func (r completionAuditRepository) ListByTask(_ context.Context, taskID string) ([]domain.CompletionAudit, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	out := make([]domain.CompletionAudit, 0)
	for _, audit := range r.storage.completionAudits {
		if audit.TaskID == taskID {
			out = append(out, audit)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt.Before(out[j].RequestedAt) })
	return out, nil
}
