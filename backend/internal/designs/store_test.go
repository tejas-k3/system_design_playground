package designs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"sysdesign/internal/engine"
)

func diagram(label string) engine.Diagram {
	return engine.Diagram{Nodes: []engine.Node{{ID: "a", Kind: engine.KAppServer, Label: label, Params: engine.Params{"threads": 4.0}}}, Edges: []engine.Edge{}, Notes: []engine.Note{}}
}

func TestPutThenGetRoundTrips(t *testing.T) {
	s, err := New("", 0)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Put(diagram("one"))
	if err != nil {
		t.Fatal(err)
	}
	if !idPattern.MatchString(id) {
		t.Fatalf("id %q is not the issued shape", id)
	}
	got, ok, err := s.Get(id)
	if err != nil || !ok || got.Nodes[0].Label != "one" {
		t.Fatalf("round trip failed: %+v ok=%v err=%v", got, ok, err)
	}
}

func TestIDsAreUniqueAndUnguessableLooking(t *testing.T) {
	s, _ := New("", 0)
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		id, _ := s.Put(diagram("x"))
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestMalformedAndUnknownIDs(t *testing.T) {
	s, _ := New(t.TempDir(), 0)
	for _, bad := range []string{"", "short", "../../etc/passwd", "UPPERCASE12345", "abc defghijklm", "aaaaaaaaaaaaaa"} {
		if _, ok, err := s.Get(bad); ok || err != ErrBadID {
			t.Errorf("Get(%q): ok=%v err=%v, want ErrBadID", bad, ok, err)
		}
	}
	if _, ok, err := s.Get("aaaaaaaaaaaaa"); ok || err != nil {
		t.Fatalf("a well-formed unknown id should be a clean miss, got ok=%v err=%v", ok, err)
	}
}

func TestSurvivesARestartWhenGivenADirectory(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir, 0)
	id, err := s.Put(diagram("kept"))
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := New(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, _ := reopened.Get(id)
	if !ok || got.Nodes[0].Label != "kept" {
		t.Fatalf("design lost across restart: ok=%v %+v", ok, got)
	}
	if reopened.Len() != 1 {
		t.Fatalf("expected 1 design after reload, got %d", reopened.Len())
	}
}

func TestOldestDesignsAreDroppedBeyondTheLimit(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir, 3)
	var ids []string
	for i := 0; i < 5; i++ {
		id, _ := s.Put(diagram(fmt.Sprint(i)))
		ids = append(ids, id)
	}
	if s.Len() != 3 {
		t.Fatalf("kept %d, want 3", s.Len())
	}
	for i, id := range ids {
		_, ok, _ := s.Get(id)
		if want := i >= 2; ok != want {
			t.Errorf("design %d present=%v want %v", i, ok, want)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) != 3 {
		t.Fatalf("disk should also hold 3 files, has %d", len(files))
	}
}

func TestIgnoresStrayFilesAndCorruptDesigns(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "aaaaaaaaaaaaa.json"), []byte("{not json"), 0o600)
	s, err := New(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get("aaaaaaaaaaaaa"); ok {
		t.Fatal("a corrupt file must read as missing")
	}
}

func TestConcurrentUse(t *testing.T) {
	s, _ := New(t.TempDir(), 50)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				id, err := s.Put(diagram("c"))
				if err != nil {
					t.Error(err)
					return
				}
				s.Get(id)
			}
		}()
	}
	wg.Wait()
	if s.Len() > 50 {
		t.Fatalf("limit exceeded: %d", s.Len())
	}
}
