// Package importer 实现各种 GPS 数据源的流式导入。
// Parser 接口允许大文件按行 emit Point，避免一次性加载到内存。
package importer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"geokeep/internal/model"
	"geokeep/internal/repo"
)

// Parser 流式解析；每解析出一条 Point 即调用 emit。
type Parser interface {
	Parse(ctx context.Context, r io.Reader, emit func(*model.Point) error) error
}

// FormatGPX 等是支持的格式 ID。
const (
	FormatGPX          = "gpx"
	FormatGeoJSON      = "geojson"
	FormatOwntracksRec = "owntracks_rec"
	FormatDawarichV1   = "dawarich_v1"
	FormatDawarichV2   = "dawarich_v2"
)

// ErrUnsupportedFormat 未知格式。
var ErrUnsupportedFormat = errors.New("不支持的导入格式")

// NewParser 工厂方法。
func NewParser(format string) (Parser, error) {
	switch format {
	case FormatGPX:
		return &gpxParser{}, nil
	case FormatGeoJSON:
		return &geoJSONParser{}, nil
	case FormatOwntracksRec:
		return &owntracksRecParser{}, nil
	case FormatDawarichV1:
		return &dawarichV1Parser{}, nil
	case FormatDawarichV2:
		return &dawarichV2Parser{}, nil
	}
	return nil, ErrUnsupportedFormat
}

// Manager 是导入任务的后台 worker（单实例 + 串行）。
type Manager struct {
	repo *repo.Repo
	dir  string
	jobs chan job
	done chan struct{}
}

type job struct {
	userID   uint
	importID uint
	format   string
	path     string
}

// NewManager 构造 Manager。dir 是导入文件落盘根目录。
func NewManager(r *repo.Repo, dir string) *Manager {
	return &Manager{repo: r, dir: dir, jobs: make(chan job, 16), done: make(chan struct{})}
}

// Start 启动 worker goroutine。
func (m *Manager) Start(ctx context.Context) {
	go m.run(ctx)
}

// Stop 等待所有任务结束。
func (m *Manager) Stop() {
	close(m.jobs)
	<-m.done
}

// Submit 投递一个导入任务到队列。
func (m *Manager) Submit(userID, importID uint, format, path string) {
	m.jobs <- job{userID: userID, importID: importID, format: format, path: path}
}

// Dir 返回落盘根目录，便于 handler 拼路径。
func (m *Manager) Dir() string { return m.dir }

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

func (m *Manager) handle(ctx context.Context, j job) {
	parser, err := NewParser(j.format)
	if err != nil {
		_ = m.repo.UpdateImport(ctx, j.userID, j.importID, map[string]any{
			"status": "failed", "error_message": err.Error(),
		})
		return
	}
	f, err := os.Open(j.path)
	if err != nil {
		_ = m.repo.UpdateImport(ctx, j.userID, j.importID, map[string]any{
			"status": "failed", "error_message": err.Error(),
		})
		return
	}
	defer f.Close()
	started := model.NowUnix()
	_ = m.repo.UpdateImport(ctx, j.userID, j.importID, map[string]any{
		"status": "running", "processing_started_at": started,
	})

	var raw, inserted, dup int
	flush := []*model.Point{}
	commit := func() {
		if len(flush) == 0 {
			return
		}
		for _, p := range flush {
			p.UserID = j.userID
			p.Source = "import:" + j.format
			p.ImportID = &j.importID
		}
		ins, du, err := m.repo.InsertPointsBatch(ctx, flush)
		if err == nil {
			inserted += ins
			dup += du
		}
		flush = flush[:0]
	}
	err = parser.Parse(ctx, f, func(p *model.Point) error {
		raw++
		flush = append(flush, p)
		if len(flush) >= 200 {
			commit()
			_ = m.repo.UpdateImport(ctx, j.userID, j.importID, map[string]any{
				"raw_points": raw, "processed": inserted, "doubles": dup,
			})
		}
		return nil
	})
	commit()
	if err != nil {
		_ = m.repo.UpdateImport(ctx, j.userID, j.importID, map[string]any{
			"raw_points": raw, "processed": inserted, "doubles": dup,
			"status": "failed", "error_message": err.Error(),
		})
		return
	}
	_ = m.repo.UpdateImport(ctx, j.userID, j.importID, map[string]any{
		"raw_points": raw, "processed": inserted, "doubles": dup,
		"status": "completed",
	})
}

// DetectFormat 根据上传文件扩展名做启发式判断；未识别返回 "".
func DetectFormat(filename string) string {
	switch {
	case hasSuffix(filename, ".gpx"):
		return FormatGPX
	case hasSuffix(filename, ".geojson"), hasSuffix(filename, ".json"):
		return FormatGeoJSON
	case hasSuffix(filename, ".rec"):
		return FormatOwntracksRec
	case hasSuffix(filename, ".tar.gz"), hasSuffix(filename, ".tgz"), hasSuffix(filename, ".zip"):
		return FormatDawarichV2
	}
	return ""
}

func hasSuffix(s, suf string) bool {
	if len(s) < len(suf) {
		return false
	}
	return s[len(s)-len(suf):] == suf
}

// Errorf 简化构造。
func Errorf(format string, args ...any) error { return fmt.Errorf(format, args...) }
