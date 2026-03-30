package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Action struct {
	Type     string `json:"type"`               // "recipe", "prompt", "notify"
	RecipeID string `json:"recipeId,omitempty"`
	Text     string `json:"text,omitempty"`
}

type Task struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`    // "polling", "monitor", "workflow", "schedule"
	AppID   string          `json:"appId"`
	Name    string          `json:"name"`
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config"`
	LastRun string          `json:"lastRun,omitempty"`
	NextRun string          `json:"nextRun,omitempty"`
}

type PollingConfig struct {
	Interval string `json:"interval"` // "30m", "1h", "5m"
	Action   Action `json:"action"`
}

type MonitorConfig struct {
	Selector  string `json:"selector"`
	TextMatch string `json:"textMatch"`
	Action    Action `json:"action"`
}

type WorkflowConfig struct {
	Trigger struct {
		Selector  string `json:"selector"`
		TextMatch string `json:"textMatch"`
	} `json:"trigger"`
	Actions []Action `json:"actions"`
}

type ScheduleConfig struct {
	Cron   string `json:"cron"`   // cron expression or simple time "09:00"
	Days   string `json:"days"`   // "mon-fri", "daily", "weekdays"
	Action Action `json:"action"`
}

var (
	taskMu     sync.RWMutex
	taskCached *TasksFile
)

type TasksFile struct {
	Tasks []Task `json:"tasks"`
}

func tasksPath() string {
	return filepath.Join(DataDir(), "tasks.json")
}

// loadTasks loads from disk on first call, caches in memory. Caller must hold taskMu.
func loadTasks() *TasksFile {
	if taskCached != nil {
		return taskCached
	}
	taskCached = &TasksFile{}
	data, err := os.ReadFile(tasksPath())
	if err != nil {
		return taskCached
	}
	json.Unmarshal(data, taskCached)
	return taskCached
}

// saveTasks writes to disk and updates cache. Caller must hold taskMu.
func saveTasks(t *TasksFile) error {
	taskCached = t
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tasksPath(), data, 0644)
}

func LoadTasks() []Task {
	taskMu.Lock()
	defer taskMu.Unlock()
	return loadTasks().Tasks
}

func SaveTasks(tasks []Task) error {
	taskMu.Lock()
	defer taskMu.Unlock()
	return saveTasks(&TasksFile{Tasks: tasks})
}

func GetTask(id string) *Task {
	taskMu.Lock()
	defer taskMu.Unlock()

	tf := loadTasks()
	for i := range tf.Tasks {
		if tf.Tasks[i].ID == id {
			return &tf.Tasks[i]
		}
	}
	return nil
}

func AddTask(task Task) error {
	taskMu.Lock()
	defer taskMu.Unlock()

	tf := loadTasks()
	for _, t := range tf.Tasks {
		if t.ID == task.ID {
			return fmt.Errorf("task with id %q already exists", task.ID)
		}
	}
	tf.Tasks = append(tf.Tasks, task)
	return saveTasks(tf)
}

func UpdateTask(task Task) error {
	taskMu.Lock()
	defer taskMu.Unlock()

	tf := loadTasks()
	for i, t := range tf.Tasks {
		if t.ID == task.ID {
			tf.Tasks[i] = task
			return saveTasks(tf)
		}
	}
	return fmt.Errorf("task with id %q not found", task.ID)
}

func RemoveTask(id string) error {
	taskMu.Lock()
	defer taskMu.Unlock()

	tf := loadTasks()
	found := false
	filtered := make([]Task, 0, len(tf.Tasks))
	for _, t := range tf.Tasks {
		if t.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		return fmt.Errorf("task with id %q not found", id)
	}
	tf.Tasks = filtered
	return saveTasks(tf)
}

func ListTasksByApp(appID string) []Task {
	taskMu.Lock()
	defer taskMu.Unlock()

	tf := loadTasks()
	var result []Task
	for _, t := range tf.Tasks {
		if t.AppID == appID {
			result = append(result, t)
		}
	}
	return result
}

func GenerateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}
