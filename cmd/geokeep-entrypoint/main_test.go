package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "default", in: nil, want: []string{"serve"}},
		{name: "keeps subcommand", in: []string{"rotate-key", "--email", "a@b.c"}, want: []string{"rotate-key", "--email", "a@b.c"}},
		{name: "drops explicit binary", in: []string{"/geokeep", "serve"}, want: []string{"serve"}},
		{name: "explicit binary defaults", in: []string{"geokeep"}, want: []string{"serve"}},
		{name: "keeps flags", in: []string{"--listen", ":9000"}, want: []string{"--listen", ":9000"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeArgs(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeArgs()=%v want %v", got, tt.want)
			}
		})
	}
}

func TestTargetIdentityEnvOverride(t *testing.T) {
	t.Setenv("GEOKEEP_RUN_UID", "1234")
	t.Setenv("GEOKEEP_RUN_GID", "2345")

	uid, gid, err := targetIdentity(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("targetIdentity 失败: %v", err)
	}
	if uid != 1234 || gid != 2345 {
		t.Fatalf("uid/gid=%d/%d want 1234/2345", uid, gid)
	}
}

func TestTargetIdentityRejectsInvalidEnv(t *testing.T) {
	t.Setenv("GEOKEEP_RUN_UID", "0")
	if _, _, err := targetIdentity(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("GEOKEEP_RUN_UID=0 应被拒绝")
	}
}

func TestDockerChownEnabled(t *testing.T) {
	t.Setenv("GEOKEEP_DOCKER_CHOWN", "false")
	if dockerChownEnabled() {
		t.Fatal("GEOKEEP_DOCKER_CHOWN=false 应关闭 chown")
	}
	os.Unsetenv("GEOKEEP_DOCKER_CHOWN")
	if !dockerChownEnabled() {
		t.Fatal("默认应开启 chown")
	}
}
