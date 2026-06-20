package memory

import (
	"context"
	"sort"

	"picoclip/internal/core/domain"
)

func (r skillRepository) Create(ctx context.Context, skill domain.Skill) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.skills[skill.ID] = skill
	return nil
}

func (r skillRepository) Get(ctx context.Context, id string) (domain.Skill, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	skill, ok := r.storage.skills[id]
	if !ok {
		return domain.Skill{}, domain.ErrNotFound
	}
	return skill, nil
}

func (r skillRepository) List(ctx context.Context, projectID string) ([]domain.Skill, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	skills := make([]domain.Skill, 0)
	for _, skill := range r.storage.skills {
		if projectID != "" && skill.ProjectID != "" && skill.ProjectID != projectID {
			continue
		}
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].CreatedAt.Before(skills[j].CreatedAt) })
	return skills, nil
}

func (r skillRepository) Update(ctx context.Context, skill domain.Skill) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.skills[skill.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.skills[skill.ID] = skill
	return nil
}

func (r skillRepository) Delete(ctx context.Context, id string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.skills[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.storage.skills, id)
	return nil
}
