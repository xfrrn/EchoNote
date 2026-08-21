package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupTemporaryFilesRemovesOnlyStaleEchoNoteFiles(t *testing.T) {
	directory := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{"echonote-source-old", "echonote-chunk-fresh.flac", "unrelated-old"} {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
		if name != "echonote-chunk-fresh.flac" {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}

	removed, err := CleanupTemporaryFiles(directory, time.Now().Add(-24*time.Hour))
	if err != nil || removed != 1 {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "echonote-source-old")); !os.IsNotExist(err) {
		t.Fatal("expected stale EchoNote file to be removed")
	}
	for _, name := range []string{"echonote-chunk-fresh.flac", "unrelated-old"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("expected %s to remain: %v", name, err)
		}
	}
}
