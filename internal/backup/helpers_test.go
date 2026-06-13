package backup_test

import "os"

func writeFile(p string, b []byte) error {
	return os.WriteFile(p, b, 0o600)
}

func read(p string) ([]byte, error) {
	return os.ReadFile(p)
}
