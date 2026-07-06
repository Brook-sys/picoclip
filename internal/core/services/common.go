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
	RuntimeBaseURL    string
}

func DefaultConfig() Config {
	return Config{
		PollInterval:      2 * time.Second,
		TaskTimeout:       30 * time.Minute,
		MaxConcurrentRuns: 2,
		MaxAttempts:       1,
		RuntimeBaseURL:    "http://127.0.0.1:8080",
	}
}
