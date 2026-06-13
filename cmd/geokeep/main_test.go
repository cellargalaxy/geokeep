package main

import (
	"os"
	"path/filepath"
	"testing"

	"geokeep/internal/config"
)

func TestPrepareDataDirsCreatesRuntimeDirs(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	cfg := &config.Config{DataDir: dataDir}

	if err := prepareDataDirs(cfg); err != nil {
		t.Fatalf("prepareDataDirs 失败: %v", err)
	}

	for _, dir := range []string{cfg.DataDir, cfg.ImportsDir(), cfg.ExportsDir(), cfg.BackupsDir()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("目录未创建 %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("路径不是目录: %s", dir)
		}
	}
}

func TestEnsureWritableDirRejectsFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(path, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}

	if err := ensureWritableDir(path); err == nil {
		t.Fatal("文件路径应被拒绝")
	}
}
