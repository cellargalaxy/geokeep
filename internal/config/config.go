package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Config 是 geokeep 服务的运行时配置。
// 所有字段都可经 CLI flag 或环境变量注入；CLI 优先级高于环境变量。
type Config struct {
	Listen      string // HTTP 监听地址，默认 :8080
	DataDir     string // 数据目录，默认 ./data
	Secret      string // Session HMAC 密钥，必填
	BasePath    string // 反代前缀，例如 /xxx；默认 "" 表示根路径
	MaxUploadMB int    // 单次上传字节上限，默认 5MB
	OSMTileURL  string // 前端瓦片 URL 模板
	Dev         bool   // 开发模式：允许 HTTP Cookie、放开 CORS
}

var basePathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~-]+(/[A-Za-z0-9._~-]+)*$`)

// Load 从命令行参数（os.Args[2:]）和环境变量解析配置。
// args 通常由 main 拆出，去掉子命令名后再传入。
func Load(args []string) (*Config, error) {
	cfg := defaults()
	fs := flag.NewFlagSet("geokeep", flag.ContinueOnError)
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "HTTP 监听地址")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "数据目录")
	fs.StringVar(&cfg.Secret, "secret", cfg.Secret, "Session HMAC 密钥")
	fs.StringVar(&cfg.BasePath, "base-path", cfg.BasePath, "反代前缀，例如 /xxx")
	fs.IntVar(&cfg.MaxUploadMB, "max-upload-mb", cfg.MaxUploadMB, "上传字节上限 MB")
	fs.StringVar(&cfg.OSMTileURL, "osm-tile-url", cfg.OSMTileURL, "OSM 瓦片 URL 模板")
	fs.BoolVar(&cfg.Dev, "dev", cfg.Dev, "开发模式：允许 HTTP Cookie、放开 CORS")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaults() *Config {
	cfg := &Config{
		Listen:      envOr("GEOKEEP_LISTEN", ":8080"),
		DataDir:     envOr("GEOKEEP_DATA_DIR", "./data"),
		Secret:      os.Getenv("GEOKEEP_SECRET"),
		BasePath:    envOr("GEOKEEP_BASE_PATH", ""),
		OSMTileURL:  envOr("GEOKEEP_OSM_TILE_URL", "https://tile.openstreetmap.org/{z}/{x}/{y}.png"),
		MaxUploadMB: envInt("GEOKEEP_MAX_UPLOAD_MB", 5),
		Dev:         envBool("GEOKEEP_DEV", false),
	}
	return cfg
}

// Validate 在启动期做硬性校验。
func (c *Config) Validate() error {
	if c.Listen == "" {
		return errors.New("listen 不可为空")
	}
	if c.DataDir == "" {
		return errors.New("data-dir 不可为空")
	}
	if c.Secret == "" {
		return errors.New("GEOKEEP_SECRET 必须设置（用于 session HMAC，长度建议 ≥ 32 字节）")
	}
	if len(c.Secret) < 16 {
		return errors.New("GEOKEEP_SECRET 长度过短（< 16 字节），不安全")
	}
	if c.MaxUploadMB <= 0 {
		return errors.New("max-upload-mb 必须 > 0")
	}
	if c.BasePath != "" {
		if !basePathPattern.MatchString(c.BasePath) {
			return fmt.Errorf("base-path 必须形如 /xxx 或 /xxx/yyy，且不带尾斜杠: %q", c.BasePath)
		}
		for _, part := range strings.Split(strings.TrimPrefix(c.BasePath, "/"), "/") {
			if part == "." || part == ".." {
				return fmt.Errorf("base-path 不允许 . 或 .. 路径段: %q", c.BasePath)
			}
		}
	}
	return nil
}

// DBPath 返回 SQLite 文件绝对路径。
func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "geokeep.db")
}

// ImportsDir / ExportsDir / BackupsDir：运行期会被自动 MkdirAll。
func (c *Config) ImportsDir() string { return filepath.Join(c.DataDir, "imports") }
func (c *Config) ExportsDir() string { return filepath.Join(c.DataDir, "exports") }
func (c *Config) BackupsDir() string { return filepath.Join(c.DataDir, "backups") }

// MaxUploadBytes 单次上传字节上限。
func (c *Config) MaxUploadBytes() int64 { return int64(c.MaxUploadMB) * 1024 * 1024 }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(os.Getenv(key))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}
