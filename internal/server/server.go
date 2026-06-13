// Package server 组装 HTTP 路由 / 中间件 / handler。
//
// 路由前缀通过 cfg.BasePath 实现子路径反代支持：
// 例如 GEOKEEP_BASE_PATH=/xxx → 所有路由变为 /xxx/api/v1/...，
// 静态 SPA 也在 /xxx/ 提供。Cookie Path 同样跟随。
package server

import (
	"net/http"

	"geokeep/internal/auth"
	"geokeep/internal/backup"
	"geokeep/internal/config"
	"geokeep/internal/db"
	"geokeep/internal/exporter"
	"geokeep/internal/importer"
	"geokeep/internal/ingest"
	"geokeep/internal/repo"
)

// Server 持有所有 handler 需要的依赖。
type Server struct {
	cfg      *config.Config
	db       *db.DB
	repo     *repo.Repo
	signer   *auth.Signer
	cookie   auth.CookieMaker
	ingestor *ingest.Ingestor
	importer *importer.Manager
	exporter *exporter.Manager
	backup   *backup.Service
}

// New 构造 Server，调用方负责调用 Handler() 注册 HTTP。
func New(cfg *config.Config, d *db.DB, r *repo.Repo) *Server {
	cm := auth.CookieMaker{Path: cfg.BasePath, Dev: cfg.Dev}
	s := &Server{
		cfg:      cfg,
		db:       d,
		repo:     r,
		signer:   auth.NewSigner(cfg.Secret),
		cookie:   cm,
		ingestor: ingest.New(r),
		importer: importer.NewManager(r, cfg.ImportsDir()),
		exporter: exporter.NewManager(r, cfg.ExportsDir()),
		backup:   backup.New(d, cfg.BackupsDir()),
	}
	return s
}

// Manager 暴露后台 worker 控制权，便于优雅停机。
func (s *Server) Manager() *importer.Manager { return s.importer }

// ExportManager 暴露导出 worker。
func (s *Server) ExportManager() *exporter.Manager { return s.exporter }

// Handler 返回挂好所有路由的根 http.Handler。
// 路由表见 README §API；BasePath 前缀在此统一拼接。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	bp := s.cfg.BasePath

	// === 上报通道：仅 api_key，跳过 CSRF ===
	mux.Handle("POST "+bp+"/api/v1/owntracks/points",
		chain(http.HandlerFunc(s.handleOwntracksIngest), s.withBodyLimit, s.requireAPIKey))
	mux.Handle("POST "+bp+"/api/v1/overland/batches",
		chain(http.HandlerFunc(s.handleOverlandIngest), s.withBodyLimit, s.requireAPIKey))

	// === 公开 ===
	mux.HandleFunc("GET "+bp+"/api/v1/bootstrap", s.handleBootstrap)
	mux.Handle("POST "+bp+"/api/v1/setup", chain(http.HandlerFunc(s.handleSetup), s.withCSRF, s.withBodyLimit))
	mux.Handle("POST "+bp+"/api/v1/auth/login", chain(http.HandlerFunc(s.handleLogin), s.withCSRF, s.withBodyLimit))
	mux.Handle("POST "+bp+"/api/v1/auth/logout", chain(http.HandlerFunc(s.handleLogout), s.withCSRF))

	// === Session 保护 ===
	authn := func(h http.HandlerFunc) http.Handler {
		return chain(http.HandlerFunc(h), s.withCSRF, s.withBodyLimit, s.requireSession)
	}
	mux.Handle("GET "+bp+"/api/v1/me", chain(http.HandlerFunc(s.handleMe), s.requireSession))
	mux.Handle("GET "+bp+"/api/v1/devices", chain(http.HandlerFunc(s.handleDevices), s.requireSession))
	mux.Handle("GET "+bp+"/api/v1/points", chain(http.HandlerFunc(s.handleQueryPoints), s.requireSession))
	mux.Handle("GET "+bp+"/api/v1/stats/summary", chain(http.HandlerFunc(s.handleSummary), s.requireSession))
	mux.Handle("POST "+bp+"/api/v1/imports", authn(s.handleCreateImport))
	mux.Handle("GET "+bp+"/api/v1/imports", chain(http.HandlerFunc(s.handleListImports), s.requireSession))
	mux.Handle("GET "+bp+"/api/v1/imports/{id}", chain(http.HandlerFunc(s.handleGetImport), s.requireSession))
	mux.Handle("POST "+bp+"/api/v1/exports", authn(s.handleCreateExport))
	mux.Handle("GET "+bp+"/api/v1/exports", chain(http.HandlerFunc(s.handleListExports), s.requireSession))
	mux.Handle("GET "+bp+"/api/v1/exports/{id}", chain(http.HandlerFunc(s.handleGetExport), s.requireSession))
	mux.Handle("GET "+bp+"/api/v1/exports/{id}/download", chain(http.HandlerFunc(s.handleDownloadExport), s.requireSession))
	mux.Handle("POST "+bp+"/api/v1/api-key/rotate", authn(s.handleRotateAPIKey))

	// === Admin ===
	admin := func(h http.HandlerFunc) http.Handler {
		return chain(http.HandlerFunc(h), s.withCSRF, s.withBodyLimit, s.requireSession, s.requireAdmin)
	}
	mux.Handle("GET "+bp+"/api/v1/backup", chain(http.HandlerFunc(s.handleBackup), s.requireSession, s.requireAdmin))
	mux.Handle("POST "+bp+"/api/v1/restore", admin(s.handleRestore))

	// === 健康检查（不带前缀） ===
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	// === 静态 SPA（含 BasePath 前缀） ===
	s.mountStatic(mux)

	// 全局中间件：日志 + recover
	return chain(mux, withLogger, withRecover)
}
