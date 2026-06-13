package config_test

import (
	"strings"
	"testing"

	"geokeep/internal/config"
)

func TestValidateBasePath(t *testing.T) {
	base := &config.Config{Listen: ":0", DataDir: t.TempDir(), Secret: strings.Repeat("x", 16), MaxUploadMB: 5}
	valid := []string{"", "/xxx", "/xxx/yyy", "/a.b_~-/c-1"}
	for _, bp := range valid {
		cfg := *base
		cfg.BasePath = bp
		if err := cfg.Validate(); err != nil {
			t.Fatalf("BasePath %q 应合法: %v", bp, err)
		}
	}
	invalid := []string{"/", "xxx", "/xxx/", "/xxx//yyy", "/../x", "/x y", "/x?y"}
	for _, bp := range invalid {
		cfg := *base
		cfg.BasePath = bp
		if err := cfg.Validate(); err == nil {
			t.Fatalf("BasePath %q 应非法", bp)
		}
	}
}
