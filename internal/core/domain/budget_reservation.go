package domain

import (
	"errors"
	"time"
)

var ErrBudgetExceeded = errors.New("budget exceeded")

type BudgetPolicyScope string

const (
	BudgetPolicyScopeGlobal    BudgetPolicyScope = "global"
	BudgetPolicyScopeWorkspace BudgetPolicyScope = "workspace"
	BudgetPolicyScopeAgent     BudgetPolicyScope = "agent"
)

type BudgetPolicyEnforcement string

const (
	BudgetPolicyEnforcementHard BudgetPolicyEnforcement = "hard"
	BudgetPolicyEnforcementWarn BudgetPolicyEnforcement = "warn"
)

type BudgetPolicy struct {
	ID              string
	Scope           BudgetPolicyScope
	ScopeID         string
	PeriodStart     time.Time
	PeriodEnd       time.Time
	TokenLimit      int
	CostLimitMicros int64
	Enforcement     BudgetPolicyEnforcement
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type BudgetAccount struct {
	PolicyID           string
	SettledTokens      int
	ReservedTokens     int
	SettledCostMicros  int64
	ReservedCostMicros int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type BudgetReservationStatus string

const (
	BudgetReservationStatusReserved BudgetReservationStatus = "reserved"
	BudgetReservationStatusSettled  BudgetReservationStatus = "settled"
)

type BudgetReservation struct {
	ID                 string
	RequestID          string
	TaskID             string
	RunID              string
	AgentID            string
	WorkspaceID        string
	TokensReserved     int
	CostMicrosReserved int64
	TokensSettled      int
	CostMicrosSettled  int64
	Status             BudgetReservationStatus
	CreatedAt          time.Time
	UpdatedAt          time.Time
	SettledAt          time.Time
}

type BudgetReservationRequest struct {
	ReservationID string
	RequestID     string
	TaskID        string
	RunID         string
	AgentID       string
	WorkspaceID   string
	Tokens        int
	CostMicros    int64
	CreatedAt     time.Time
}

type BudgetReservationSettlement struct {
	ReservationID string
	Tokens        int
	CostMicros    int64
	SettledAt     time.Time
}
