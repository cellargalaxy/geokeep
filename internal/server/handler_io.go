package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"geokeep/internal/auth"
	"geokeep/internal/exporter"
	"geokeep/internal/importer"
	"geokeep/internal/model"
	"geokeep/internal/repo"
)

// handleCreateImport 接收 multipart 上传，落盘 + 创建任务 + 投递 worker。
func (s *Server) handleCreateImport(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes()); err != nil {
		http.Error(w, "bad multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "缺少 file 字段", http.StatusBadRequest)
		return
	}
	defer file.Close()
	format := r.FormValue("format")
	if format == "" {
		format = importer.DetectFormat(header.Filename)
	}
	if format == "" {
		http.Error(w, "无法识别格式，请显式指定 format", http.StatusBadRequest)
		return
	}
	if _, err := importer.NewParser(format); err != nil {
		http.Error(w, "不支持的格式: "+format, http.StatusBadRequest)
		return
	}

	// 落 Import 行（先得 ID 再拼路径，避免用户原文件名拼路径产生穿越）
	im := &model.Import{
		UserID: uid,
		Name:   safeFilename(header.Filename),
		Source: format,
		Status: "pending",
	}
	if err := s.repo.CreateImport(r.Context(), im); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	subdir := filepath.Join(s.importer.Dir(), strconv.FormatUint(uint64(im.ID), 10))
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		http.Error(w, "internal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	dst := filepath.Join(subdir, "raw"+extOf(header.Filename))
	out, err := os.Create(dst)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		http.Error(w, "internal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	out.Close()
	if err := s.repo.UpdateImport(r.Context(), uid, im.ID, map[string]any{"file_path": dst}); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	s.importer.Submit(uid, im.ID, format, dst)
	writeJSON(w, http.StatusAccepted, map[string]any{"import_id": im.ID, "status": "pending"})
}

func (s *Server) handleGetImport(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	im, err := s.repo.GetImport(r.Context(), uid, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, im)
}

func (s *Server) handleListImports(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	xs, err := s.repo.ListImports(r.Context(), uid)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imports": xs})
}

type createExportReq struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	StartAt int64  `json:"start_at"`
	EndAt   int64  `json:"end_at"`
}

func (s *Server) handleCreateExport(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	var req createExportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Format == "" {
		req.Format = exporter.FormatGeoJSON
	}
	if _, err := exporter.NewWriter(req.Format); err != nil {
		http.Error(w, "不支持的格式: "+req.Format, http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		req.Name = "export-" + req.Format
	}
	ex := &model.Export{
		UserID:     uid,
		Name:       safeFilename(req.Name),
		FileFormat: req.Format,
		Status:     "pending",
		StartAt:    req.StartAt,
		EndAt:      req.EndAt,
	}
	if err := s.repo.CreateExport(r.Context(), ex); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	s.exporter.Submit(uid, ex.ID, req.Format, req.StartAt, req.EndAt)
	writeJSON(w, http.StatusAccepted, map[string]any{"export_id": ex.ID, "status": "pending"})
}

func (s *Server) handleGetExport(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	ex, err := s.repo.GetExport(r.Context(), uid, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

func (s *Server) handleListExports(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	xs, err := s.repo.ListExports(r.Context(), uid)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"exports": xs})
}

func (s *Server) handleDownloadExport(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	ex, err := s.repo.GetExport(r.Context(), uid, id)
	if err != nil || ex.Status != "completed" || ex.FilePath == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// Content-Disposition 让浏览器按导出格式后缀命名下载，便于用户识别。
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeFilename(filepath.Base(ex.FilePath))+`"`)
	http.ServeFile(w, r, ex.FilePath)
}

// handleBackup 触发 VACUUM INTO 并流式吐文件。
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="geokeep-backup.db"`)
	if _, err := s.backup.StreamBackup(r.Context(), w); err != nil {
		http.Error(w, "backup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleRestore 上传 .db 文件到落盘 + 写 pending_restore 标记。
// MVP 不做在线热替换；用户必须重启服务由启动期完成切换。
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes()); err != nil {
		http.Error(w, "bad multipart", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "缺少 file 字段", http.StatusBadRequest)
		return
	}
	defer file.Close()
	uploadPath := filepath.Join(s.cfg.BackupsDir(), "pending-restore.db")
	if err := os.MkdirAll(filepath.Dir(uploadPath), 0o700); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	out, err := os.Create(uploadPath)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	out.Close()
	// 写标记，启动期消费
	flag := filepath.Join(s.cfg.DataDir, ".pending_restore")
	if err := os.WriteFile(flag, []byte(uploadPath), 0o600); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "pending",
		"note":   "请重启服务以完成恢复",
	})
}

func pathID(r *http.Request, name string) (uint, error) {
	v := r.PathValue(name)
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil || n == 0 {
		return 0, errors.New("invalid id")
	}
	return uint(n), nil
}

// safeFilename 去掉路径分隔符，避免目录穿越。
func safeFilename(name string) string {
	if name == "" {
		return "file"
	}
	cleaned := filepath.Base(name)
	if cleaned == "." || cleaned == "/" || cleaned == ".." {
		return "file"
	}
	return cleaned
}

func extOf(name string) string {
	ext := filepath.Ext(name)
	switch ext {
	case ".gz", ".bz2":
		if base := filepath.Ext(name[:len(name)-len(ext)]); base == ".tar" {
			return ".tar" + ext
		}
	}
	if ext == "" {
		return ""
	}
	return ext
}
