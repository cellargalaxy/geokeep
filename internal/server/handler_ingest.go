package server

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"geokeep/internal/auth"
	"geokeep/internal/ingest"
)

// handleOwntracksIngest 接收 OwnTracks _type=location 单条 payload。
// 响应固定为 "[]"，与 OwnTracks HTTP 模式约定一致（MVP 不下发好友/cmd）。
func (s *Server) handleOwntracksIngest(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	if uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	ins, dup, err := s.ingestor.SaveOwnTracks(r.Context(), uid, raw)
	if err != nil {
		if errors.Is(err, ingest.ErrOwnTracksNotLocation) {
			// 非 location（transition/waypoint/...）记日志后照样回 200 "[]"
			slog.Debug("owntracks non-location", "user_id", uid)
			respondOwnTracks(w)
			return
		}
		slog.Error("owntracks save", "err", err, "user_id", uid)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	slog.Info("ingest", "source", "owntracks", "user_id", uid, "inserted", ins, "dup", dup)
	respondOwnTracks(w)
}

func respondOwnTracks(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("[]"))
}

// handleOverlandIngest 接收 Overland 批量上报。
// 响应必须 {"result":"ok"}，否则客户端会本地堆积重传。
func (s *Server) handleOverlandIngest(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	if uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	ins, dup, err := s.ingestor.SaveOverland(r.Context(), uid, raw)
	if err != nil {
		slog.Error("overland save", "err", err, "user_id", uid)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	slog.Info("ingest", "source", "overland", "user_id", uid, "inserted", ins, "dup", dup)
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}
