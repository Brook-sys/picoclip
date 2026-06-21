package memory

import (
	"testing"

	"picoclip/internal/adapters/storage/storagetest"
	"picoclip/internal/core/ports"
)

func TestMemoryStorageContract(t *testing.T) {
	storagetest.RunStorageContract(t, func(t *testing.T) ports.Storage {
		t.Helper()
		return NewStorage()
	})
}
