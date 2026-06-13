package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"geokeep/internal/auth"
	"geokeep/internal/model"
	"geokeep/internal/repo"
)

type userRow struct {
	Email string
	Hash  string
}

type exportRow struct {
	ID     uint
	UserID uint
	Format string
}

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func createDirect(t *testing.T, r *repo.Repo, u *userRow) {
	t.Helper()
	apiKey, _ := auth.GenerateAPIKey()
	user := &model.User{Email: u.Email, PasswordHash: u.Hash, APIKey: apiKey, Settings: "{}"}
	if err := r.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
}

func insertExport(t *testing.T, r *repo.Repo, ex *exportRow) {
	t.Helper()
	m := &model.Export{UserID: ex.UserID, Name: "x", FileFormat: ex.Format, Status: "pending"}
	if err := r.CreateExport(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	ex.ID = m.ID
}

func login(t *testing.T, ts *httptest.Server, email, pw string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": pw})
	resp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("登录失败: %d", resp.StatusCode)
	}
	return resp
}

func itoa(n uint) string { return strconv.FormatUint(uint64(n), 10) }
