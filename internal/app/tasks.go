package app

import (
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TaskState represents the current state of a task
type TaskState int

const (
	// TaskPending indicates the task has been registered but not started
	TaskPending TaskState = iota
	// TaskRunning indicates the task is currently executing
	TaskRunning
	// TaskFinished indicates the task completed successfully
	TaskFinished
	// TaskError indicates the task failed with an error
	TaskError
)

// Task represents an asynchronous operation with progress tracking
type Task struct {
	// ID is a unique identifier for the task
	ID string

	// Description is a human-readable description shown while running
	Description string

	// SuccessMessage is shown when the task completes successfully
	SuccessMessage string

	// State is the current state of the task
	State TaskState

	// Error is set if the task fails
	Error error

	// StartTime is when the task started running
	StartTime time.Time

	// FinishTime is when the task completed (success or error)
	FinishTime time.Time
}

// Duration returns how long the task has been running or took to complete
func (t *Task) Duration() time.Duration {
	if t.State == TaskRunning {
		return time.Since(t.StartTime)
	}
	if !t.FinishTime.IsZero() {
		return t.FinishTime.Sub(t.StartTime)
	}
	return 0
}

// TaskStartedMsg is sent when a task begins execution
type TaskStartedMsg struct {
	TaskID string
}

// TaskFinishedMsg is sent when a task completes (successfully or with error)
type TaskFinishedMsg struct {
	TaskID string
	Error  error
}

// TaskManager tracks asynchronous operations and provides a unified
// interface for managing loading states across the application.
//
// This replaces scattered boolean flags (hubsLoading, playbackInitializing, etc.)
// with a centralized task registry that can be queried and displayed.
type TaskManager struct {
	tasks map[string]*Task
	mu    sync.RWMutex
}

// NewTaskManager creates a new task manager
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]*Task),
	}
}

// RegisterTask creates a new pending task and returns it.
// The task will not start until StartTask is called.
func (tm *TaskManager) RegisterTask(id, description, successMessage string) *Task {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task := &Task{
		ID:             id,
		Description:    description,
		SuccessMessage: successMessage,
		State:          TaskPending,
	}
	tm.tasks[id] = task
	return task
}

// StartTask marks a task as running and returns a command that sends TaskStartedMsg.
// If the task doesn't exist, it creates a new one.
func (tm *TaskManager) StartTask(id, description, successMessage string) tea.Cmd {
	tm.mu.Lock()
	task, exists := tm.tasks[id]
	if !exists {
		task = &Task{
			ID:             id,
			Description:    description,
			SuccessMessage: successMessage,
			State:          TaskRunning,
			StartTime:      time.Now(),
		}
		tm.tasks[id] = task
	} else {
		task.State = TaskRunning
		task.StartTime = time.Now()
	}
	tm.mu.Unlock()

	return func() tea.Msg {
		return TaskStartedMsg{TaskID: id}
	}
}

// FinishTask marks a task as complete (success or error) and returns a command
func (tm *TaskManager) FinishTask(id string, err error) tea.Cmd {
	tm.mu.Lock()
	task, exists := tm.tasks[id]
	if exists {
		task.FinishTime = time.Now()
		if err != nil {
			task.State = TaskError
			task.Error = err
		} else {
			task.State = TaskFinished
		}
	}
	tm.mu.Unlock()

	return func() tea.Msg {
		return TaskFinishedMsg{
			TaskID: id,
			Error:  err,
		}
	}
}

// IsRunning returns true if the specified task is currently running
func (tm *TaskManager) IsRunning(id string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	task, exists := tm.tasks[id]
	return exists && task.State == TaskRunning
}

// GetTask returns the task with the given ID, or nil if not found
func (tm *TaskManager) GetTask(id string) *Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.tasks[id]
}

// ActiveTasks returns all currently running tasks
func (tm *TaskManager) ActiveTasks() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var active []*Task
	for _, task := range tm.tasks {
		if task.State == TaskRunning {
			active = append(active, task)
		}
	}
	return active
}

// AllTasks returns all tasks (running, completed, or failed)
func (tm *TaskManager) AllTasks() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tasks := make([]*Task, 0, len(tm.tasks))
	for _, task := range tm.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// Clear removes all tasks from the manager
func (tm *TaskManager) Clear() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.tasks = make(map[string]*Task)
}

// ClearCompleted removes all finished and errored tasks, keeping only running ones
func (tm *TaskManager) ClearCompleted() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for id, task := range tm.tasks {
		if task.State == TaskFinished || task.State == TaskError {
			delete(tm.tasks, id)
		}
	}
}

// HasActiveTasks returns true if there are any running tasks
func (tm *TaskManager) HasActiveTasks() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	for _, task := range tm.tasks {
		if task.State == TaskRunning {
			return true
		}
	}
	return false
}
