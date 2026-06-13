package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"

	"geokeep/internal/web"
)

// mountStatic 挂载嵌入的前端资源；
// index.html 中的 `{{BASE_PATH}}` 占位符会被替换为 cfg.BasePath，
// 由前端 JS 通过 `window.__GEOKEEP_BASE__` 读取，确保子路径反代下所有 fetch/资源 URL 正确。
func (s *Server) mountStatic(mux *http.ServeMux) {
	staticFS, err := fs.Sub(web.FS, "static")
	if err != nil {
		panic(err)
	}
	bp := s.cfg.BasePath
	rootPath := bp + "/"
	if bp == "" {
		rootPath = "/"
	}

	// 静态资源：/{base}/static/...
	mux.Handle("GET "+bp+"/static/", http.StripPrefix(bp+"/static/", http.FileServer(http.FS(staticFS))))

	// 首页：返回模板替换后的 index.html
	mux.HandleFunc("GET "+rootPath, func(w http.ResponseWriter, r *http.Request) {
		// 子路径下避免「漏到上级」：仅当 path 完全是 rootPath 或 rootPath+index*
		if !strings.HasPrefix(r.URL.Path, rootPath) {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			http.Error(w, "index missing", http.StatusInternalServerError)
			return
		}
		out := bytes.ReplaceAll(data, []byte("{{BASE_PATH}}"), []byte(bp))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	})
}
