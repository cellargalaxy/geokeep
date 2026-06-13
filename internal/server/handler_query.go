package server

import (
	"net/http"
	"strconv"

	"geokeep/internal/auth"
	"geokeep/internal/repo"
)

// handleQueryPoints 在时间窗 + 设备 + 抽样后返回精简列。
func (s *Server) handleQueryPoints(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	q := repo.PointQuery{
		UserID: uid,
		From:   atoi64(r.URL.Query().Get("from")),
		To:     atoi64(r.URL.Query().Get("to")),
		Limit:  atoiOr(r.URL.Query().Get("limit"), 50000),
		Sample: atoiOr(r.URL.Query().Get("sample"), 1),
	}
	if q.Limit > 200000 {
		q.Limit = 200000
	}
	for _, v := range r.URL.Query()["device_id"] {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q.DeviceIDs = append(q.DeviceIDs, uint(n))
		}
	}
	pts, err := s.repo.QueryPoints(r.Context(), q)
	if err != nil {
		http.Error(w, "internal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 输出精简结构，不返回 raw_data
	out := make([]map[string]any, 0, len(pts))
	for _, p := range pts {
		row := map[string]any{
			"id": p.ID, "ts": p.Timestamp, "lat": p.Latitude, "lon": p.Longitude,
		}
		if p.DeviceID != nil {
			row["device_id"] = *p.DeviceID
		}
		if p.Altitude != nil {
			row["alt"] = *p.Altitude
		}
		if p.Velocity != "" {
			row["vel"] = p.Velocity
		}
		if p.Course != nil {
			row["cog"] = *p.Course
		}
		if p.Accuracy != nil {
			row["acc"] = *p.Accuracy
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "points": out})
}

// handleDevices 列出用户自己的设备。
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	ds, err := s.repo.ListDevices(r.Context(), uid)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": ds})
}

// handleSummary 返回简单统计。
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	q := repo.PointQuery{
		UserID: uid,
		From:   atoi64(r.URL.Query().Get("from")),
		To:     atoi64(r.URL.Query().Get("to")),
	}
	pts, err := s.repo.QueryPoints(r.Context(), q)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	dist := totalDistanceMeters(pts)
	writeJSON(w, http.StatusOK, map[string]any{"count": len(pts), "distance_m": dist})
}

// handleHealthz 探活：DB 可达 + 服务存活。
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := s.db.DB.DB()
	if err == nil {
		err = sqlDB.PingContext(r.Context())
	}
	if err != nil {
		http.Error(w, "db down", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
