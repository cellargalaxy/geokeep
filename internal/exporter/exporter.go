// Package exporter 实现多种 GPS 数据格式的导出。
// 与 importer 一一对应；exporter 写盘后由 handler 提供下载链接。
package exporter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"geokeep/internal/model"
	"geokeep/internal/repo"
)

// Writer 在收到 Open 后开始写入流；Write 单条；Close 终止。
type Writer interface {
	Open(w io.Writer) error
	Write(p model.Point) error
	Close() error
	Extension() string
}

const (
	FormatGeoJSON       = "geojson"
	FormatGPX           = "gpx"
	FormatOwnTracksJSON = "owntracks_json"
	FormatDawarichV2    = "dawarich_v2"
)

// ErrUnsupportedFormat 未知格式。
var ErrUnsupportedFormat = errors.New("不支持的导出格式")

// NewWriter 工厂方法。
func NewWriter(format string) (Writer, error) {
	switch format {
	case FormatGeoJSON:
		return &geoJSONWriter{}, nil
	case FormatGPX:
		return &gpxWriter{}, nil
	case FormatOwnTracksJSON:
		return &owntracksJSONWriter{}, nil
	case FormatDawarichV2:
		return &dawarichV2Writer{}, nil
	}
	return nil, ErrUnsupportedFormat
}

// Manager 单 worker 顺序处理导出任务。
type Manager struct {
	repo *repo.Repo
	dir  string
	jobs chan exportJob
	done chan struct{}
}

type exportJob struct {
	userID   uint
	exportID uint
	format   string
	startAt  int64
	endAt    int64
}

// NewManager 构造导出管理器。
func NewManager(r *repo.Repo, dir string) *Manager {
	return &Manager{repo: r, dir: dir, jobs: make(chan exportJob, 16), done: make(chan struct{})}
}

// Dir 返回根目录。
func (m *Manager) Dir() string { return m.dir }

// Start 启动 worker。
func (m *Manager) Start(ctx context.Context) {
	go m.run(ctx)
}

// Stop 等待所有任务结束。
func (m *Manager) Stop() {
	close(m.jobs)
	<-m.done
}

// Submit 投递导出任务。
func (m *Manager) Submit(userID, exportID uint, format string, startAt, endAt int64) {
	m.jobs <- exportJob{userID: userID, exportID: exportID, format: format, startAt: startAt, endAt: endAt}
}

func (m *Manager) run(ctx context.Context) {
	defer close(m.done)
	for {
		select {
		case <-ctx.Done():
			return
		case j, ok := <-m.jobs:
			if !ok {
				return
			}
			m.handle(ctx, j)
		}
	}
}

func (m *Manager) handle(ctx context.Context, j exportJob) {
	writer, err := NewWriter(j.format)
	if err != nil {
		_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
		return
	}
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
		return
	}
	subdir := filepath.Join(m.dir, strconv.FormatUint(uint64(j.exportID), 10))
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
		return
	}
	path := filepath.Join(subdir, "export."+writer.Extension())
	f, err := os.Create(path)
	if err != nil {
		_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
		return
	}
	started := model.NowUnix()
	_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "running", "processing_started_at": started})

	if err := writer.Open(f); err != nil {
		f.Close()
		_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
		return
	}
	pts, err := m.repo.QueryPoints(ctx, repo.PointQuery{UserID: j.userID, From: j.startAt, To: j.endAt})
	if err != nil {
		writer.Close()
		f.Close()
		_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
		return
	}
	for _, p := range pts {
		if err := writer.Write(p); err != nil {
			writer.Close()
			f.Close()
			_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
			return
		}
	}
	if err := writer.Close(); err != nil {
		f.Close()
		_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
		return
	}
	if err := f.Close(); err != nil {
		_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
		return
	}
	st, err := os.Stat(path)
	if err != nil {
		_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{"status": "failed", "error_message": err.Error()})
		return
	}
	_ = m.repo.UpdateExport(ctx, j.userID, j.exportID, map[string]any{
		"status":    "completed",
		"file_path": path,
		"file_size": st.Size(),
	})
}

// Errorf 简化构造。
func Errorf(format string, args ...any) error { return fmt.Errorf(format, args...) }
