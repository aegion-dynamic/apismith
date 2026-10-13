package request

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// HistoryItem is a locally stored, non-secret record of a request.
type HistoryItem struct {
	ID          int               `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	Environment string            `json:"environment"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	PathParams  map[string]string `json:"path_params,omitempty"`
	Query       map[string]string `json:"query,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	BodyType    string            `json:"body_type,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	AuthMode    AuthMode          `json:"auth_mode"`
	Status      int               `json:"status"`
	DurationMS  int64             `json:"duration_ms"`
	OK          bool              `json:"ok"`
	Error       string            `json:"error,omitempty"`
}

// History is a ring of recent requests. JWTs are never stored.
// When a path is set, the ring is written beside the environments file
// and reloaded on the next start.
type History struct {
	mu      sync.RWMutex
	items   []HistoryItem
	nextID  int
	maxSize int
	path    string
}

// NewHistory keeps the last maxSize requests.
func NewHistory(maxSize int) *History {
	if maxSize <= 0 {
		maxSize = 50
	}
	return &History{maxSize: maxSize, nextID: 1}
}

// LoadFile reads a previously saved ring. A missing file is not an error.
// A corrupt file returns an error and does not become the save path, so a
// later request cannot overwrite it.
func (h *History) LoadFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			h.mu.Lock()
			h.path = path
			h.mu.Unlock()
			return nil
		}
		return err
	}
	if len(bytesTrim(b)) == 0 {
		h.mu.Lock()
		h.path = path
		h.mu.Unlock()
		return nil
	}
	var items []HistoryItem
	if err := json.Unmarshal(b, &items); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = items
	h.path = path
	maxID := 0
	for _, it := range h.items {
		if it.ID > maxID {
			maxID = it.ID
		}
	}
	h.nextID = maxID + 1
	if h.nextID < 1 {
		h.nextID = 1
	}
	h.trimLocked()
	return nil
}

func bytesTrim(b []byte) []byte {
	i := 0
	j := len(b)
	for i < j && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\t' || b[j-1] == '\r') {
		j--
	}
	return b[i:j]
}

// Add appends an item and evicts the oldest if needed.
func (h *History) Add(item HistoryItem) HistoryItem {
	h.mu.Lock()
	defer h.mu.Unlock()
	item.ID = h.nextID
	h.nextID++
	if item.Timestamp.IsZero() {
		item.Timestamp = time.Now()
	}
	item.Headers = stripSensitive(item.Headers)
	h.items = append(h.items, item)
	h.trimLocked()
	h.persistLocked()
	return item
}

func (h *History) trimLocked() {
	if len(h.items) > h.maxSize {
		h.items = h.items[len(h.items)-h.maxSize:]
	}
}

func (h *History) persistLocked() {
	if h.path == "" {
		return
	}
	b, err := json.MarshalIndent(h.items, "", "  ")
	if err != nil {
		log.Printf("history save: %v", err)
		return
	}
	b = append(b, '\n')
	tmp := h.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Printf("history save: %v", err)
		return
	}
	if err := os.Rename(tmp, h.path); err != nil {
		log.Printf("history save: %v", err)
	}
}

// List returns newest-first copies.
func (h *History) List() []HistoryItem {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]HistoryItem, len(h.items))
	for i := range h.items {
		out[len(h.items)-1-i] = h.items[i]
	}
	return out
}

// Get returns a history item by id.
func (h *History) Get(id int) (HistoryItem, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, it := range h.items {
		if it.ID == id {
			return it, true
		}
	}
	return HistoryItem{}, false
}

// Clear drops all history.
func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = nil
	h.persistLocked()
}
