package devin

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type ExportResult struct {
	Path      string `json:"path"`
	FileCount int    `json:"file_count"`
	Size      int64  `json:"size"`
	Message   string `json:"message"`
}

func ExportChats(destinationDir string) (*ExportResult, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return nil, err
	}
	source := filepath.Join(paths.UserDataDir, "User", "acp-messages")
	return exportChatsFrom(source, destinationDir, time.Now())
}

func exportChatsFrom(source, destinationDir string, now time.Time) (*ExportResult, error) {
	entries, err := os.ReadDir(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("尚未找到 Devin 聊天记录")
		}
		return nil, err
	}
	if strings.TrimSpace(destinationDir) == "" {
		home, _ := os.UserHomeDir()
		destinationDir = filepath.Join(home, "Downloads")
	}
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return nil, err
	}
	name := "Devin-Chats-" + now.Format("20060102-150405") + ".zip"
	target := filepath.Join(destinationDir, name)
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(f)
	closeWithError := func(cause error) (*ExportResult, error) {
		_ = zw.Close()
		_ = f.Close()
		_ = os.Remove(target)
		return nil, cause
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		low := strings.ToLower(entry.Name())
		if strings.HasSuffix(low, ".db") || strings.HasSuffix(low, ".db-wal") || strings.HasSuffix(low, ".db-shm") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return closeWithError(fmt.Errorf("尚未找到可导出的 Devin 聊天记录"))
	}
	for _, fileName := range files {
		if err := addFileToZip(zw, filepath.Join(source, fileName), filepath.Join("acp-messages", fileName)); err != nil {
			return closeWithError(err)
		}
	}
	manifest := map[string]any{
		"format": "devin-byok-chat-backup-v1", "created_at": now.UTC().Format(time.RFC3339),
		"platform": runtime.GOOS, "file_count": len(files), "files": files,
		"note": "Raw Devin ACP message databases. Close Devin before export for the strongest snapshot consistency.",
	}
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	mw, err := zw.Create("manifest.json")
	if err != nil {
		return closeWithError(err)
	}
	if _, err := mw.Write(append(manifestBytes, '\n')); err != nil {
		return closeWithError(err)
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		_ = os.Remove(target)
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(target)
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	return &ExportResult{Path: target, FileCount: len(files), Size: info.Size(), Message: "聊天记录已导出到下载目录"}, nil
}

func addFileToZip(zw *zip.Writer, source, archiveName string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(archiveName)
	header.Method = zip.Deflate
	out, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return err
}
