package devin

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExportChatsFromIncludesDatabaseSidecarsAndManifest(t *testing.T) {
	source := filepath.Join(t.TempDir(), "acp-messages")
	destination := t.TempDir()
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"chat.db": "db", "chat.db-wal": "wal", "chat.db-shm": "shm", "ignore.txt": "ignore",
	} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	result, err := exportChatsFrom(source, destination, time.Date(2026, 8, 14, 12, 34, 56, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.FileCount != 3 || filepath.Base(result.Path) != "Devin-Chats-20260814-123456.zip" {
		t.Fatalf("unexpected result: %#v", result)
	}
	zr, err := zip.OpenReader(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	want := map[string]bool{
		"acp-messages/chat.db": false, "acp-messages/chat.db-wal": false,
		"acp-messages/chat.db-shm": false, "manifest.json": false,
	}
	for _, file := range zr.File {
		if _, ok := want[file.Name]; ok {
			want[file.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("archive is missing %s", name)
		}
	}
}

func TestExportChatsFromRejectsEmptySource(t *testing.T) {
	source := t.TempDir()
	if _, err := exportChatsFrom(source, t.TempDir(), time.Now()); err == nil {
		t.Fatal("expected an error for an empty source")
	}
}
