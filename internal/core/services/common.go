package services

import "time"

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}

type Config struct {
	PollInterval      time.Duration
	TaskTimeout       time.Duration
	MaxConcurrentRuns int
	MaxAttempts       int
}

func DefaultConfig() Config {
	return Config{
		PollInterval:      2 * time.Second,
		TaskTimeout:       2 * time.Minute,
		MaxConcurrentRuns: 2,
		MaxAttempts:       1,
	}
}
