package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type AppConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"` // webview2, pwa, electron, webapp, native
	LaunchCmd   string            `json:"launch_cmd,omitempty"`
	URL         string            `json:"url,omitempty"`
	CDPPort     int               `json:"cdp_port,omitempty"`
	Headless    bool              `json:"headless,omitempty"`
	AutoStart   bool              `json:"auto_start,omitempty"`
	Credentials map[string]string `json:"credentials,omitempty"`
	Recipes     []string          `json:"recipes,omitempty"`
	Tasks       []string          `json:"tasks,omitempty"`
}

type AppsFile struct {
	Apps []AppConfig `json:"apps"`
}

var (
	mu     sync.Mutex
	cached *AppsFile
)

func appsPath() string {
	return filepath.Join(DataDir(), "apps.json")
}

// loadApps loads from disk on first call, caches in memory. Caller must hold mu.
func loadApps() *AppsFile {
	if cached != nil {
		return cached
	}
	cached = &AppsFile{}
	data, err := os.ReadFile(appsPath())
	if err != nil {
		return cached
	}
	json.Unmarshal(data, cached)
	return cached
}

// saveApps writes to disk and updates cache. Caller must hold mu.
func saveApps(a *AppsFile) error {
	cached = a
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(appsPath(), data, 0644)
}

func LoadApps() *AppsFile {
	mu.Lock()
	defer mu.Unlock()
	return loadApps()
}

func SaveApps(a *AppsFile) error {
	mu.Lock()
	defer mu.Unlock()
	return saveApps(a)
}

func GetApp(id string) *AppConfig {
	mu.Lock()
	defer mu.Unlock()

	apps := loadApps()
	for i := range apps.Apps {
		if apps.Apps[i].ID == id {
			return &apps.Apps[i]
		}
	}
	return nil
}

func AddApp(app AppConfig) error {
	mu.Lock()
	defer mu.Unlock()

	apps := loadApps()
	for _, a := range apps.Apps {
		if a.ID == app.ID {
			return fmt.Errorf("app with id %q already exists", app.ID)
		}
	}
	apps.Apps = append(apps.Apps, app)
	return saveApps(apps)
}

func RemoveApp(id string) error {
	mu.Lock()
	defer mu.Unlock()

	apps := loadApps()
	found := false
	filtered := make([]AppConfig, 0, len(apps.Apps))
	for _, a := range apps.Apps {
		if a.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, a)
	}
	if !found {
		return fmt.Errorf("app with id %q not found", id)
	}
	apps.Apps = filtered
	return saveApps(apps)
}

func ListApps() []AppConfig {
	mu.Lock()
	defer mu.Unlock()
	return loadApps().Apps
}
