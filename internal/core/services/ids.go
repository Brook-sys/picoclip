package services

import (
	"fmt"
	"sync/atomic"
	"time"
)

type TimeIDGenerator struct {
	seq uint64
}

func (g *TimeIDGenerator) NewID(prefix string) string {
	n := atomic.AddUint64(&g.seq, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), n)
}
