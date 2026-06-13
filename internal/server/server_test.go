package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"geokeep/internal/config"
	"geokeep/internal/db"
	"geokeep/internal/repo"
	"geokeep/internal/server"
)

// makeServer 拉起一个含真实 SQLite 的 server，用于端到端测试。
func makeServer(t *testing.T, basePath string) (*httptest.Server, *repo.Repo) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Listen:      ":0",
		DataDir:     dir,
		Secret:      "0123456789abcdef0123456789abcdef",
		BasePath:    basePath,
		MaxUploadMB: 5,
		OSMTileURL:  "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
		Dev:         true, // 关闭 CSRF/Secure 限制方便测试
	}
	d, err := db.Open(filepath.Join(dir, "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	r := repo.New(d)
	s := server.New(cfg, d, r)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() {
		ts.Close()
		d.Close()
	})
	return ts, r
}

func TestBootstrap_Initialized(t *testing.T) {
	ts, _ := makeServer(t, "")
	resp, err := http.Get(ts.URL + "/api/v1/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["initialized"] != false {
		t.Fatalf("初始应未初始化: %v", got)
	}
}

func TestSetup_Then_NotAllowedAgain(t *testing.T) {
	ts, _ := makeServer(t, "")
	body := bytes.NewReader([]byte(`{"email":"a@b.c","password":"abcdefghij"}`))
	resp, err := http.Post(ts.URL+"/api/v1/setup", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("setup 失败: %d", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["api_key"] == "" {
		t.Fatal("应返回 api_key")
	}
	// 第二次应 410
	resp2, _ := http.Post(ts.URL+"/api/v1/setup", "application/json", strings.NewReader(`{"email":"x@y.z","password":"abcdefghij"}`))
	if resp2.StatusCode != http.StatusGone {
		t.Fatalf("第二次 setup 应 410: %d", resp2.StatusCode)
	}
}

func TestOwntracks_Ingest_With_APIKey(t *testing.T) {
	ts, r := makeServer(t, "")
	// 先 setup
	resp, _ := http.Post(ts.URL+"/api/v1/setup", "application/json", strings.NewReader(`{"email":"a@b.c","password":"abcdefghij"}`))
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	apiKey, _ := got["api_key"].(string)
	if apiKey == "" {
		t.Fatal("无 api_key")
	}
	// 上报
	payload := `{"_type":"location","lat":1.0,"lon":2.0,"tst":100,"tid":"X"}`
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/owntracks/points?api_key="+apiKey, strings.NewReader(payload))
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("上报失败: %d", resp2.StatusCode)
	}
	// 校验落库
	n, _ := r.CountPoints(req.Context(), 1)
	if n != 1 {
		t.Fatalf("应有 1 条点位: %d", n)
	}
}

func TestOwntracks_BadAPIKey(t *testing.T) {
	ts, _ := makeServer(t, "")
	// 不 setup，直接非法 key
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/owntracks/points?api_key=wrong", strings.NewReader(`{"_type":"location","lat":1,"lon":2,"tst":1}`))
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("应 401: %d", resp.StatusCode)
	}
}

func TestSession_Required_For_Query(t *testing.T) {
	ts, _ := makeServer(t, "")
	resp, _ := http.Get(ts.URL + "/api/v1/points?from=0&to=999999")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录应 401: %d", resp.StatusCode)
	}
}

func TestBasePath_Mounting(t *testing.T) {
	ts, _ := makeServer(t, "/xxx")
	// 根路径应 404
	resp, _ := http.Get(ts.URL + "/api/v1/bootstrap")
	if resp.StatusCode == 200 {
		t.Fatal("根路径不应有 bootstrap")
	}
	// 子路径应可达
	resp2, err := http.Get(ts.URL + "/xxx/api/v1/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("子路径 bootstrap 应 200: %d", resp2.StatusCode)
	}
}

// 越权回归：B 登录后查 A 的 export 应 404。
func TestCrossUser_ExportBlocked(t *testing.T) {
	ts, r := makeServer(t, "")
	// 创建 A
	_, _ = http.Post(ts.URL+"/api/v1/setup", "application/json", strings.NewReader(`{"email":"a@b","password":"abcdefghij"}`))
	// 直接通过 repo 注入 B（绕过 setup，因为 setup 只允许一次）
	hash := mustHash(t, "abcdefghij")
	uB := &userRow{Email: "b@b", Hash: hash}
	createDirect(t, r, uB)
	// 创建 A 的 export
	exA := &exportRow{UserID: 1, Format: "geojson"}
	insertExport(t, r, exA)
	// B 登录拿 cookie
	loginResp := login(t, ts, "b@b", "abcdefghij")
	cookie := loginResp.Cookies()[0]
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/exports/"+itoa(exA.ID), nil)
	req.AddCookie(cookie)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("B 不应访问 A 的 export，期望 404，实际 %d", resp.StatusCode)
	}
}
