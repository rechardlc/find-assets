package server

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/find-assets/scanner/internal/exporter"
)

// TaskStatus 异步任务的生命周期阶段。
type TaskStatus string

const (
	StatusPending TaskStatus = "pending"
	StatusRunning TaskStatus = "running"
	StatusDone    TaskStatus = "done"
	StatusFailed  TaskStatus = "failed"
)

// ProgressEvent 通过 SSE 推送的进度帧。
type ProgressEvent struct {
	Done    int64  `json:"done"`
	Total   int64  `json:"total"`
	Stage   string `json:"stage"` // listing / scanning / finished
	Message string `json:"message,omitempty"`
}

// Task 一个异步扫描任务的运行时状态。
type Task struct {
	ID         string           `json:"id"`
	Mode       string           `json:"mode"`
	Status     TaskStatus       `json:"status"`
	Total      int64            `json:"total"`
	Done       int64            `json:"done"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt time.Time        `json:"finished_at,omitempty"`
	Err        string           `json:"error,omitempty"`
	Report     *exporter.Report `json:"-"`

	mu          sync.Mutex
	subscribers []chan ProgressEvent
	finished    chan struct{}
}

func newTask(id, mode string) *Task {
	return &Task{
		ID:        id,
		Mode:      mode,
		Status:    StatusPending,
		StartedAt: time.Now(),
		finished:  make(chan struct{}),
	}
}

// publish 广播一帧进度（非阻塞，慢订阅者会被丢帧）。
func (t *Task) publish(ev ProgressEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, ch := range t.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Subscribe 订阅进度，返回带缓冲的接收通道；调用方使用完应忽略，不必显式注销。
func (t *Task) Subscribe() chan ProgressEvent {
	ch := make(chan ProgressEvent, 16)
	t.mu.Lock()
	t.subscribers = append(t.subscribers, ch)
	t.mu.Unlock()
	return ch
}

// Finished 在任务进入终态后立即关闭。
func (t *Task) Finished() <-chan struct{} { return t.finished }

func (t *Task) markFinished() {
	select {
	case <-t.finished:
	default:
		close(t.finished)
	}
}

// TaskStore 进程内的任务表。
type TaskStore struct {
	mu     sync.RWMutex
	tasks  map[string]*Task
	idSeed atomic.Uint64
}

func NewTaskStore() *TaskStore {
	return &TaskStore{tasks: make(map[string]*Task)}
}

func (s *TaskStore) Put(t *Task) {
	s.mu.Lock()
	s.tasks[t.ID] = t
	s.mu.Unlock()
}

func (s *TaskStore) Get(id string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks[id]
}
