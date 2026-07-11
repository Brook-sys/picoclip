package ports

import (
	"context"
	"time"

	"picoclip/internal/core/domain"
)

// MemoryDocumentKind identifies the source and intended use of a memory
// document without coupling the core to an adapter-specific schema.
type MemoryDocumentKind string

const (
	MemoryDocumentKindRun     MemoryDocumentKind = "run"
	MemoryDocumentKindTask    MemoryDocumentKind = "task"
	MemoryDocumentKindMessage MemoryDocumentKind = "message"
	MemoryDocumentKindCustom  MemoryDocumentKind = "custom"
)

// MemoryDocument is the provider-neutral unit stored and searched by memory
// adapters. Content is the canonical text. Embedding is optional so local
// adapters may use lexical search while vector adapters may persist vectors.
// EmbeddingModel identifies the vector space (model plus relevant version);
// adapters must not compare embeddings from different non-empty vector spaces.
//
// Store implementations must treat ID as an idempotency key and replace the
// existing document when the same ID is stored again. Metadata must remain
// query-independent and must not contain credentials or other secrets.
type MemoryDocument struct {
	ID             string
	Kind           MemoryDocumentKind
	WorkspaceID    string
	AgentID        string
	TaskID         string
	RunID          string
	Content        string
	Embedding      []float32
	EmbeddingModel string
	Metadata       map[string]string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// MemoryQuery describes a semantic or lexical similarity search. At least one
// of Text or Embedding should be supplied. EmbeddingModel is required when an
// Embedding is supplied and restricts comparison to that vector space.
// Non-empty scope fields and Kinds are conjunctive filters; callers should
// always provide WorkspaceID when the task belongs to a workspace. Limit <= 0
// lets the provider apply its documented default. MinScore is inclusive and
// uses the normalized [0,1] score returned by Search.
type MemoryQuery struct {
	Text           string
	Embedding      []float32
	EmbeddingModel string
	WorkspaceID    string
	AgentID        string
	TaskID         string
	RunID          string
	Kinds          []MemoryDocumentKind
	Limit          int
	MinScore       float64
}

// MemorySearchResult pairs a document with its normalized similarity score.
// Search results must be ordered by descending Score. Equal scores must be
// ordered by Document.ID ascending so behavior is portable across adapters.
type MemorySearchResult struct {
	Document MemoryDocument
	Score    float64
}

// MemoryProvider is the core boundary for pluggable agent memory. Adapters may
// use lexical matching, embeddings, or a remote vector index, but must preserve
// the filtering, ordering, score, and idempotency semantics documented here.
type MemoryProvider interface {
	// Store creates or replaces a provider-neutral memory document by ID.
	// Implementations must honor context cancellation and must not partially
	// publish a document when the operation returns an error.
	Store(ctx context.Context, document MemoryDocument) error
	// Search returns the most similar documents matching all supplied filters.
	// It must honor context cancellation and return no score outside [0,1].
	Search(ctx context.Context, query MemoryQuery) ([]MemorySearchResult, error)
	// ContextForTask returns bounded, prompt-ready context relevant to a task and
	// agent. Providers decide formatting and size limits and should return an
	// empty string, not an error, when no relevant memory exists.
	ContextForTask(ctx context.Context, task domain.Task, agent domain.Agent) (string, error)
	// SaveRun extracts and idempotently stores the durable memory represented by
	// a completed or failed run. The run ID is the natural idempotency key.
	SaveRun(ctx context.Context, task domain.Task, run domain.Run) error
}
