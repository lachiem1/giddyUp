package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasLocalDBFilesReturnsFalseWhenMissing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "giddyup.db")
	exists, err := hasLocalDBFiles(path)
	if err != nil {
		t.Fatalf("hasLocalDBFiles() unexpected error: %v", err)
	}
	if exists {
		t.Fatal("hasLocalDBFiles() = true, want false")
	}
}

func TestHasLocalDBFilesDetectsPrimaryDB(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "giddyup.db")
	if err := os.WriteFile(path, []byte("db"), 0o600); err != nil {
		t.Fatalf("write db file: %v", err)
	}

	exists, err := hasLocalDBFiles(path)
	if err != nil {
		t.Fatalf("hasLocalDBFiles() unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("hasLocalDBFiles() = false, want true")
	}
}

func TestHasLocalDBFilesDetectsWalOrShm(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "giddyup.db")
	if err := os.WriteFile(path+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatalf("write wal file: %v", err)
	}

	exists, err := hasLocalDBFiles(path)
	if err != nil {
		t.Fatalf("hasLocalDBFiles() unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("hasLocalDBFiles() = false, want true")
	}
}
