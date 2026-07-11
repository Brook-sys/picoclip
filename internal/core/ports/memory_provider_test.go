package ports

import (
	"context"
	"testing"

	"picoclip/internal/core/domain"
)

type memoryProviderStub struct {
	stored MemoryDocument
	query  MemoryQuery
}

var _ MemoryProvider = (*memoryProviderStub)(nil)

func (stub *memoryProviderStub) Store(_ context.Context, document MemoryDocument) error {
	stub.stored = document
	return nil
}

func (stub *memoryProviderStub) Search(_ context.Context, query MemoryQuery) ([]MemorySearchResult, error) {
	stub.query = query
	return []MemorySearchResult{{Document: stub.stored, Score: 0.75}}, nil
}

func (*memoryProviderStub) ContextForTask(context.Context, domain.Task, domain.Agent) (string, error) {
	return "context", nil
}

func (*memoryProviderStub) SaveRun(context.Context, domain.Task, domain.Run) error {
	return nil
}

func TestMemoryProviderSupportsStoreAndScopedSimilaritySearch(t *testing.T) {
	provider := &memoryProviderStub{}
	document := MemoryDocument{
		ID:             "run_1",
		Kind:           MemoryDocumentKindRun,
		WorkspaceID:    "workspace_1",
		TaskID:         "task_1",
		RunID:          "run_1",
		Content:        "completed task context",
		Embedding:      []float32{0.25, 0.75},
		EmbeddingModel: "test-embedding-v1",
	}

	if err := provider.Store(context.Background(), document); err != nil {
		t.Fatalf("store memory document: %v", err)
	}

	query := MemoryQuery{
		Text:           "task context",
		Embedding:      []float32{0.25, 0.75},
		EmbeddingModel: "test-embedding-v1",
		WorkspaceID:    "workspace_1",
		TaskID:         "task_1",
		RunID:          "run_1",
		Kinds:          []MemoryDocumentKind{MemoryDocumentKindRun},
		Limit:          5,
		MinScore:       0.5,
	}
	results, err := provider.Search(context.Background(), query)
	if err != nil {
		t.Fatalf("search memory documents: %v", err)
	}

	if provider.stored.ID != document.ID {
		t.Fatalf("stored document ID = %q, want %q", provider.stored.ID, document.ID)
	}
	if provider.query.RunID != query.RunID || provider.query.TaskID != query.TaskID {
		t.Fatalf("query scope = task %q run %q, want task %q run %q", provider.query.TaskID, provider.query.RunID, query.TaskID, query.RunID)
	}
	if provider.query.EmbeddingModel != document.EmbeddingModel {
		t.Fatalf("query embedding model = %q, want %q", provider.query.EmbeddingModel, document.EmbeddingModel)
	}
	if len(results) != 1 || results[0].Document.ID != document.ID || results[0].Score != 0.75 {
		t.Fatalf("results = %#v, want stored document with score 0.75", results)
	}
}
