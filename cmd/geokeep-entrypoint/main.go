package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	defaultDataDir = "/data"
	geokeepPath    = "/geokeep"
	nonrootUID     = 65532
	nonrootGID     = 65532
)

func main() {
	dataDir := envOr("GEOKEEP_DATA_DIR", defaultDataDir)
	args := normalizeArgs(os.Args[1:])

	uid, gid, err := targetIdentity(dataDir)
	if err != nil {
		exitf("解析运行用户失败: %v", err)
	}
	if err := prepareDataDir(dataDir, uid, gid); err != nil {
		exitf("准备数据目录失败: %v", err)
	}
	if os.Geteuid() == 0 {
		if err := dropPrivileges(uid, gid); err != nil {
			exitf("降权失败: %v", err)
		}
	}

	argv := append([]string{geokeepPath}, args...)
	if err := syscall.Exec(geokeepPath, argv, os.Environ()); err != nil {
		exitf("执行 geokeep 失败: %v", err)
	}
}

func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return []string{"serve"}
	}
	if filepath.Base(args[0]) == "geokeep" {
		args = args[1:]
	}
	if len(args) == 0 {
		return []string{"serve"}
	}
	return args
}

func targetIdentity(dataDir string) (int, int, error) {
	uid, gid := nonrootUID, nonrootGID
	if info, err := os.Stat(dataDir); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != 0 {
			uid = int(stat.Uid)
			gid = int(stat.Gid)
		}
	}

	var err error
	if v := os.Getenv("GEOKEEP_RUN_UID"); v != "" {
		uid, err = parsePositiveID("GEOKEEP_RUN_UID", v)
		if err != nil {
			return 0, 0, err
		}
	}
	if v := os.Getenv("GEOKEEP_RUN_GID"); v != "" {
		gid, err = parsePositiveID("GEOKEEP_RUN_GID", v)
		if err != nil {
			return 0, 0, err
		}
	}
	return uid, gid, nil
}

func parsePositiveID(name, value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s 必须是正整数: %q", name, value)
	}
	return n, nil
}

func prepareDataDir(dataDir string, uid, gid int) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	if os.Geteuid() != 0 || !dockerChownEnabled() {
		return nil
	}
	return chownRecursive(dataDir, uid, gid)
}

func dockerChownEnabled() bool {
	v := strings.ToLower(os.Getenv("GEOKEEP_DOCKER_CHOWN"))
	return v != "0" && v != "false" && v != "no" && v != "off"
}

func chownRecursive(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := os.Lchown(path, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", path, err)
		}
		return nil
	})
}

func dropPrivileges(uid, gid int) error {
	if err := syscall.Setgroups([]int{}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid: %w", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid: %w", err)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "geokeep-entrypoint: "+format+"\n", args...)
	os.Exit(1)
}
