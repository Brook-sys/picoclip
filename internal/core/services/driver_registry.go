package services

import (
	"sync"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type DriverRegistry struct {
	mu      sync.RWMutex
	drivers map[domain.AgentType]ports.Driver
}

func NewDriverRegistry() *DriverRegistry {
	return &DriverRegistry{drivers: make(map[domain.AgentType]ports.Driver)}
}

func (r *DriverRegistry) Register(driver ports.Driver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drivers[driver.Type()] = driver
}

func (r *DriverRegistry) Get(agentType domain.AgentType) (ports.Driver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	driver, ok := r.drivers[agentType]
	return driver, ok
}
