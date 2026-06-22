package store

import "time"

// TaskID uniquely identifies a task.
type TaskID string

// TaskKind categorises a task (download, start, query, etc.).
type TaskKind string

// TaskStatus tracks the lifecycle of a task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskRunning    TaskStatus = "running"
	TaskCompleted  TaskStatus = "completed"
	TaskFailed     TaskStatus = "failed"
	TaskCancelled  TaskStatus = "cancelled"
)

// TaskProgress holds completion metrics for a running task.
type TaskProgress struct {
	Percent float64
	Message string
}

// TaskEvent is a timestamped event emitted by a task.
type TaskEvent struct {
	At      time.Time
	Message string
}

// StatePath is a dot-separated path into AppState, e.g. "chat.sessions.s1".
type StatePath string

// Task represents a unit of scheduled work. Tasks form a tree via ParentID.
type Task struct {
	ID        TaskID
	ParentID  TaskID
	Kind      TaskKind
	Status    TaskStatus
	StatePath StatePath
	Progress  TaskProgress
	Summary   string
	Error     string
	Events    []TaskEvent
	CreatedAt time.Time
	UpdatedAt time.Time
}
