package repo_test

import (
	"context"
	"path/filepath"
	"testing"

	"geokeep/internal/db"
	"geokeep/internal/model"
	"geokeep/internal/repo"
)

func newRepo(t *testing.T) (*repo.Repo, func()) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	return repo.New(d), func() { d.Close() }
}

func TestUser_CRUD(t *testing.T) {
	r, cleanup := newRepo(t)
	defer cleanup()
	ctx := context.Background()

	n, err := r.UserCount(ctx)
	if err != nil || n != 0 {
		t.Fatalf("初始 UserCount 期望 0: %d %v", n, err)
	}

	u := &model.User{Email: "a@b.c", PasswordHash: "h", APIKey: "key1", Admin: true, Settings: "{}"}
	if err := r.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("自增 ID 未填充")
	}

	got, err := r.GetUserByAPIKey(ctx, "key1")
	if err != nil || got.ID != u.ID {
		t.Fatalf("GetUserByAPIKey: %v %v", got, err)
	}

	// 轮换 API key
	if err := r.UpdateAPIKey(ctx, u.ID, "key2"); err != nil {
		t.Fatalf("UpdateAPIKey: %v", err)
	}
	if _, err := r.GetUserByAPIKey(ctx, "key1"); err != repo.ErrNotFound {
		t.Fatalf("旧 key 应失效，实际 %v", err)
	}
	if _, err := r.GetUserByAPIKey(ctx, "key2"); err != nil {
		t.Fatalf("新 key 应可用: %v", err)
	}
}

// 防越权回归：B 用 A 的 import_id 取数据应返回 ErrNotFound。
func TestImport_Cross_User_Blocked(t *testing.T) {
	r, cleanup := newRepo(t)
	defer cleanup()
	ctx := context.Background()
	a := &model.User{Email: "a@x", PasswordHash: "h", APIKey: "ka", Settings: "{}"}
	b := &model.User{Email: "b@x", PasswordHash: "h", APIKey: "kb", Settings: "{}"}
	if err := r.CreateUser(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateUser(ctx, b); err != nil {
		t.Fatal(err)
	}

	im := &model.Import{UserID: a.ID, Name: "a.gpx", Source: "gpx", Status: "pending", FilePath: "/tmp/x"}
	if err := r.CreateImport(ctx, im); err != nil {
		t.Fatal(err)
	}

	if _, err := r.GetImport(ctx, b.ID, im.ID); err != repo.ErrNotFound {
		t.Fatalf("B 不应取到 A 的 import: %v", err)
	}
	// A 自己取应成功
	if _, err := r.GetImport(ctx, a.ID, im.ID); err != nil {
		t.Fatalf("A 取自己的 import 失败: %v", err)
	}
	// B 不能 update A 的 import
	err := r.UpdateImport(ctx, b.ID, im.ID, map[string]any{"status": "failed"})
	if err != repo.ErrNotFound {
		t.Fatalf("B 不能 update A 的 import: %v", err)
	}
}

// 防越权回归：Point 跨用户隔离。
func TestPoint_Cross_User_Isolation(t *testing.T) {
	r, cleanup := newRepo(t)
	defer cleanup()
	ctx := context.Background()
	a := &model.User{Email: "a@x", PasswordHash: "h", APIKey: "ka", Settings: "{}"}
	b := &model.User{Email: "b@x", PasswordHash: "h", APIKey: "kb", Settings: "{}"}
	_ = r.CreateUser(ctx, a)
	_ = r.CreateUser(ctx, b)
	pa := &model.Point{UserID: a.ID, Timestamp: 1000, Latitude: 1.0, Longitude: 2.0, Source: "owntracks"}
	pb := &model.Point{UserID: b.ID, Timestamp: 1000, Latitude: 1.0, Longitude: 2.0, Source: "owntracks"}
	_, _ = r.InsertPoint(ctx, pa)
	_, _ = r.InsertPoint(ctx, pb)
	// 同坐标同时间不同用户互不影响（唯一约束含 user_id）
	if pa.ID == 0 || pb.ID == 0 {
		t.Fatal("两用户应能各自插入")
	}

	pts, err := r.QueryPoints(ctx, repo.PointQuery{UserID: a.ID, From: 0, To: 9999})
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 1 || pts[0].UserID != a.ID {
		t.Fatalf("A 只能查到自己的点: %v", pts)
	}
}

func TestPoint_UniqueDedup(t *testing.T) {
	r, cleanup := newRepo(t)
	defer cleanup()
	ctx := context.Background()
	u := &model.User{Email: "a@x", PasswordHash: "h", APIKey: "ka", Settings: "{}"}
	_ = r.CreateUser(ctx, u)
	p1 := &model.Point{UserID: u.ID, Timestamp: 1, Latitude: 1, Longitude: 2, Source: "owntracks"}
	p2 := &model.Point{UserID: u.ID, Timestamp: 1, Latitude: 1, Longitude: 2, Source: "owntracks"}
	ins, _ := r.InsertPoint(ctx, p1)
	if !ins {
		t.Fatal("首次应插入成功")
	}
	ins, err := r.InsertPoint(ctx, p2)
	if err != nil {
		t.Fatalf("唯一冲突应被吃掉: %v", err)
	}
	if ins {
		t.Fatal("重复点应识别为 dup")
	}
}

func TestPoint_QuerySample(t *testing.T) {
	r, cleanup := newRepo(t)
	defer cleanup()
	ctx := context.Background()
	u := &model.User{Email: "a@x", PasswordHash: "h", APIKey: "ka", Settings: "{}"}
	_ = r.CreateUser(ctx, u)
	for i := 0; i < 10; i++ {
		p := &model.Point{UserID: u.ID, Timestamp: int64(i), Latitude: float64(i), Longitude: 1, Source: "owntracks"}
		_, _ = r.InsertPoint(ctx, p)
	}
	pts, err := r.QueryPoints(ctx, repo.PointQuery{UserID: u.ID, From: 0, To: 9999, Sample: 3})
	if err != nil {
		t.Fatal(err)
	}
	// 抽样后剩约 4 个（i=0,3,6,9）
	if len(pts) != 4 {
		t.Fatalf("Sample=3 期望 4 条，实际 %d", len(pts))
	}
}

func TestDevice_Upsert(t *testing.T) {
	r, cleanup := newRepo(t)
	defer cleanup()
	ctx := context.Background()
	u := &model.User{Email: "a@x", PasswordHash: "h", APIKey: "ka", Settings: "{}"}
	_ = r.CreateUser(ctx, u)
	d1, _ := r.UpsertDevice(ctx, u.ID, "iPhone", "owntracks")
	d2, _ := r.UpsertDevice(ctx, u.ID, "iPhone", "owntracks")
	if d1.ID != d2.ID {
		t.Fatalf("同名设备应复用: %d %d", d1.ID, d2.ID)
	}
}

// QueryPoints 不带 user_id 必须报错（防御性测试）。
func TestQueryPoints_RequiresUserID(t *testing.T) {
	r, cleanup := newRepo(t)
	defer cleanup()
	_, err := r.QueryPoints(context.Background(), repo.PointQuery{UserID: 0})
	if err == nil {
		t.Fatal("UserID=0 必须报错")
	}
}
