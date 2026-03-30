package recipes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasaugustodev/cdp-mcp/cdp"
)

// Recipe is a saved sequence of recorded steps.
type Recipe struct {
	Name  string            `json:"name"`
	Steps []cdp.RecordEvent `json:"steps"`
}

// recipesDir returns the path to the recipes directory.
func recipesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cdp-mcp", "recipes")
}

// ensureDir creates the recipes directory if it doesn't exist.
func ensureDir() error {
	return os.MkdirAll(recipesDir(), 0o755)
}

// sanitizeName ensures the recipe name is safe for filesystem use.
func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "..", "_")
	if !strings.HasSuffix(name, ".json") {
		name = name + ".json"
	}
	return name
}

// SaveRecipe persists a recipe to disk.
func SaveRecipe(name string, steps []cdp.RecordEvent) error {
	if err := ensureDir(); err != nil {
		return fmt.Errorf("create recipes dir: %w", err)
	}

	recipe := Recipe{
		Name:  name,
		Steps: steps,
	}
	data, err := json.MarshalIndent(recipe, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recipe: %w", err)
	}

	path := filepath.Join(recipesDir(), sanitizeName(name))
	return os.WriteFile(path, data, 0o644)
}

// LoadRecipe loads a recipe from disk.
func LoadRecipe(name string) (*Recipe, error) {
	path := filepath.Join(recipesDir(), sanitizeName(name))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recipe %q: %w", name, err)
	}

	var recipe Recipe
	if err := json.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("parse recipe %q: %w", name, err)
	}
	return &recipe, nil
}

// ListRecipes returns all saved recipe names.
func ListRecipes() []string {
	entries, err := os.ReadDir(recipesDir())
	if err != nil {
		return nil
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			names = append(names, strings.TrimSuffix(name, ".json"))
		}
	}
	return names
}

// DeleteRecipe removes a recipe from disk.
func DeleteRecipe(name string) error {
	path := filepath.Join(recipesDir(), sanitizeName(name))
	return os.Remove(path)
}
