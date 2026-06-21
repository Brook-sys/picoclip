package services

type NoopLogger struct{}

func (NoopLogger) Debug(msg string, args ...any) {}
func (NoopLogger) Info(msg string, args ...any)  {}
func (NoopLogger) Warn(msg string, args ...any)  {}
func (NoopLogger) Error(msg string, args ...any) {}
