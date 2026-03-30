package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasaugustodev/cdp-mcp/config"
	"github.com/lucasaugustodev/cdp-mcp/recipes"
)

// agentDir returns ~/.claude/agents/, creating it if it doesn't exist.
func agentDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".claude", "agents")
	os.MkdirAll(dir, 0o755)
	return dir
}

// GenerateAgentFile creates a markdown agent file for the given app config.
func GenerateAgentFile(app config.AppConfig) error {
	headless := "no"
	if app.Headless {
		headless = "yes"
	}

	// Build recipes section
	var recipeLines string
	if len(app.Recipes) > 0 {
		for _, r := range app.Recipes {
			recipeLines += fmt.Sprintf("- %s\n", r)
		}
	} else {
		recipeLines = "No recipes configured.\n"
	}

	// Build tasks section
	var taskLines string
	if len(app.Tasks) > 0 {
		for _, t := range app.Tasks {
			taskLines += fmt.Sprintf("- %s\n", t)
		}
	} else {
		taskLines = "No active tasks.\n"
	}

	content := fmt.Sprintf(`# %s Agent

You control %s via CDP MCP tools (cdp-tools server).

## Connection
Connect first: `+"`"+`connect {"target": "%s"}`+"`"+`

## How to interact
- Use `+"`"+`find`+"`"+` with `+"`"+`click=true`+"`"+` for fast clicks: `+"`"+`find {"query": "Button text", "click": true}`+"`"+`
- Use `+"`"+`js`+"`"+` for precise DOM: `+"`"+`js {"code": "document.querySelector('...').click()"}`+"`"+`
- Use `+"`"+`screenshot`+"`"+` to see the current screen
- Use `+"`"+`type_text`+"`"+` to type: `+"`"+`type_text {"text": "hello"}`+"`"+`
- Use `+"`"+`press_key`+"`"+` for Enter/Tab/Escape: `+"`"+`press_key {"key": "Enter"}`+"`"+`

## Available Recipes
%s
## Active Tasks
%s
## Notes
- App type: %s
- Headless: %s
- Always verify actions with screenshot
- Respond in the user's language
`, app.Name, app.Name, app.Name,
		recipeLines, taskLines,
		app.Type, headless)

	path := filepath.Join(agentDir(), app.ID+".md")
	return os.WriteFile(path, []byte(content), 0o644)
}

// RegenerateAgentFile loads an app's config and regenerates its agent file.
func RegenerateAgentFile(appID string) error {
	app := config.GetApp(appID)
	if app == nil {
		return fmt.Errorf("app %q not found", appID)
	}

	// Enrich recipes from the recipes store, filtered by what the app declares
	allRecipes := recipes.ListRecipes()
	if len(app.Recipes) == 0 && len(allRecipes) > 0 {
		// If no explicit filter, include all available recipes
		app.Recipes = allRecipes
	} else if len(app.Recipes) > 0 {
		// Filter: keep only recipes that actually exist on disk
		var filtered []string
		recipeSet := make(map[string]bool)
		for _, r := range allRecipes {
			recipeSet[strings.ToLower(r)] = true
		}
		for _, r := range app.Recipes {
			if recipeSet[strings.ToLower(r)] {
				filtered = append(filtered, r)
			}
		}
		app.Recipes = filtered
	}

	return GenerateAgentFile(*app)
}

// RemoveAgentFile deletes the agent markdown file for the given app.
func RemoveAgentFile(appID string) error {
	path := filepath.Join(agentDir(), appID+".md")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove agent file: %w", err)
	}
	return nil
}
