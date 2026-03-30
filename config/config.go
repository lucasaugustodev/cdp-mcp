package config

import (
	"os"
	"path/filepath"
)

func DataDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cdp-mcp")
	os.MkdirAll(dir, 0755)
	return dir
}
