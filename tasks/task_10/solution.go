package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type TaskRepo interface {
	Create(title string) (Task, error)
	Get(id string) (Task, bool)
	List(done bool) []Task
	SetDone(id string, done bool) (Task, error)
}

type Clock interface{ Now() time.Time }

var (
	ErrNotFound     = errors.New("task not found")
	ErrInvalidTitle = errors.New("invalid title")
	nextRepoID      atomic.Uint64
)

type inMemoryTaskRepo struct {
	mu     sync.RWMutex
	clock  Clock
	repoID uint64
	seq    uint64
	tasks  map[string]Task
}

func NewInMemoryTaskRepo(clock Clock) TaskRepo {
	return &inMemoryTaskRepo{clock: clock, repoID: nextRepoID.Add(1), tasks: make(map[string]Task)}
}

func (r *inMemoryTaskRepo) Create(title string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, ErrInvalidTitle
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	task := Task{
		ID:        strconv.FormatUint(r.repoID, 10) + ":" + strconv.FormatUint(r.seq, 10),
		Title:     title,
		UpdatedAt: r.clock.Now(),
	}
	r.tasks[task.ID] = task
	return task, nil
}

func (r *inMemoryTaskRepo) Get(id string) (Task, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[id]
	return task, ok
}

func (r *inMemoryTaskRepo) List(done bool) []Task {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Task, 0)
	for _, task := range r.tasks {
		if task.Done == done {
			result = append(result, task)
		}
	}
	return result
}

func (r *inMemoryTaskRepo) SetDone(id string, done bool) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}
	if task.Done != done {
		task.Done = done
		task.UpdatedAt = r.clock.Now()
		r.tasks[id] = task
	}
	return task, nil
}

type httpHandler struct{ repo TaskRepo }

func NewHTTPHandler(repo TaskRepo) http.Handler {
	return &httpHandler{repo: repo}
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/tasks" {
		switch r.Method {
		case http.MethodPost:
			h.handleCreate(w, r)
		case http.MethodGet:
			h.handleList(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/tasks/") {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/tasks/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r, id)
	case http.MethodPatch:
		h.handlePatch(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *httpHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title *string `json:"title"`
	}
	if err := decodeStrictJSON(r.Body, &input); err != nil || input.Title == nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	task, err := h.repo.Create(*input.Title)
	if err != nil {
		if errors.Is(err, ErrInvalidTitle) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h *httpHandler) handleGet(w http.ResponseWriter, r *http.Request, id string) {
	task, ok := h.repo.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *httpHandler) handleList(w http.ResponseWriter, r *http.Request) {
	done := false
	if values, exists := r.URL.Query()["done"]; exists {
		if len(values) != 1 || (values[0] != "true" && values[0] != "false") {
			http.Error(w, "invalid done parameter", http.StatusBadRequest)
			return
		}
		done = values[0] == "true"
	}
	tasks := h.repo.List(done)
	if tasks == nil {
		tasks = []Task{}
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].UpdatedAt.Equal(tasks[j].UpdatedAt) {
			return tasks[i].ID < tasks[j].ID
		}
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})
	writeJSON(w, http.StatusOK, tasks)
}

func (h *httpHandler) handlePatch(w http.ResponseWriter, r *http.Request, id string) {
	var input struct {
		Done *bool `json:"done"`
	}
	if err := decodeStrictJSON(r.Body, &input); err != nil || input.Done == nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	task, err := h.repo.SetDone(id, *input.Done)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func decodeStrictJSON(r io.Reader, v any) error {
	decoder := json.NewDecoder(r)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("extra JSON value")
		}
		return err
	}
	if len(raw) == 0 || raw[0] != '{' {
		return errors.New("expected JSON object")
	}
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	return strict.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
