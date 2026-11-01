// Package designs keeps shared diagrams so a design can be sent to someone as a link.
// There are no accounts: an unguessable id is the capability to read a design.
package designs

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"sysdesign/internal/engine"
)

// DefaultMax bounds how many designs are kept; the oldest are dropped first.
const DefaultMax = 5000

var idPattern = regexp.MustCompile(`^[a-z2-7]{13}$`)

// ErrBadID is returned for an id that could not have been issued by this store.
var ErrBadID = errors.New("designs: malformed id")

// Store holds designs in memory and, when given a directory, also on disk so they survive restarts.
type Store struct {
	mu    sync.Mutex
	dir   string
	max   int
	items map[string]engine.Diagram
	order []string // oldest first
}

// New opens a store. An empty dir keeps everything in memory only; max <= 0 means DefaultMax.
func New(dir string, max int) (*Store, error) {
	if max <= 0 {
		max = DefaultMax
	}
	s := &Store{dir: dir, max: max, items: map[string]engine.Diagram{}}
	if dir == "" {
		return s, nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type aged struct {
		id string
		t  int64
	}
	var found []aged
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || !idPattern.MatchString(id) {
			continue
		}
		if info, err := e.Info(); err == nil {
			found = append(found, aged{id, info.ModTime().UnixNano()})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].t < found[j].t })
	for _, f := range found {
		s.order = append(s.order, f.id)
	}
	s.trim()
	return s, nil
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])), nil // 13 chars
}

// Put stores a diagram and returns its id.
func (s *Store) Put(d engine.Diagram) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir != "" {
		data, err := json.Marshal(d)
		if err != nil {
			return "", err
		}
		tmp := filepath.Join(s.dir, id+".tmp")
		if err := os.WriteFile(tmp, data, 0o640); err != nil {
			return "", err
		}
		if err := os.Rename(tmp, filepath.Join(s.dir, id+".json")); err != nil {
			return "", err
		}
	}
	s.items[id] = d
	s.order = append(s.order, id)
	s.trim()
	return id, nil
}

// Get returns the design with this id, if there is one.
func (s *Store) Get(id string) (engine.Diagram, bool, error) {
	if !idPattern.MatchString(id) {
		return engine.Diagram{}, false, ErrBadID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.items[id]; ok {
		return d, true, nil
	}
	if s.dir == "" || !s.known(id) {
		return engine.Diagram{}, false, nil
	}
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil {
		return engine.Diagram{}, false, nil
	}
	var d engine.Diagram
	if err := json.Unmarshal(data, &d); err != nil {
		return engine.Diagram{}, false, nil
	}
	s.items[id] = d
	return d, true, nil
}

// Len reports how many designs are kept.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.order)
}

func (s *Store) known(id string) bool {
	for _, o := range s.order {
		if o == id {
			return true
		}
	}
	return false
}

// trim drops the oldest designs beyond the limit. The caller holds the lock.
func (s *Store) trim() {
	for len(s.order) > s.max {
		old := s.order[0]
		s.order = s.order[1:]
		delete(s.items, old)
		if s.dir != "" {
			// best effort: the design is already forgotten in memory, and a leftover file is harmless
			_ = os.Remove(filepath.Join(s.dir, old+".json"))
		}
	}
}
