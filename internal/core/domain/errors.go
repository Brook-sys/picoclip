package domain

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrInvalidInput       = errors.New("invalid input")
	ErrNoPendingTasks     = errors.New("no pending tasks")
	ErrConflict           = errors.New("conflict")
	ErrForbidden          = errors.New("forbidden")
	ErrDriverUnavailable  = errors.New("driver unavailable")
	ErrRuntimeUnavailable = errors.New("runtime unavailable")
)
