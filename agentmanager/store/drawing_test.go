package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agentmanager"
	"agentmanager/store"
)

func TestModelAndDrawingMeta(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	r, err := st.Create(agentmanager.Claude)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []func() error{
		func() error { return r.SetModel("opus") },
		func() error { return r.SetModel("opus") },
		func() error { return r.SetDrawing("plate.dxf") },
		func() error { return r.AddUser("hi") },
		func() error { return r.SetModel("sonnet") },
		func() error { return r.SetDrawing("plate-v2.dxf") },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	if m := r.Meta(); m.Model != "sonnet" || m.Drawing != "plate-v2.dxf" {
		t.Fatalf("live meta %+v", m)
	}
	r.Close()
	id := r.Meta().ID
	got, err := st.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if m := got.Meta(); m.Model != "sonnet" || m.Drawing != "plate-v2.dxf" || m.Title != "hi" {
		t.Fatalf("loaded meta %+v", m)
	}
	ms, _ := st.List()
	if len(ms) != 1 || ms[0].Model != "sonnet" || ms[0].Drawing != "plate-v2.dxf" {
		t.Fatalf("listed %+v", ms)
	}
	if err := got.SetModel(""); err != nil {
		t.Fatal(err)
	}
	got.Close()
	again, _ := st.Load(id)
	if again.Meta().Model != "" {
		t.Fatalf("model not reset: %+v", again.Meta())
	}
	b, _ := os.ReadFile(filepath.Join(st.Dir(), id+".jsonl"))
	if n := strings.Count(string(b), `"k":"model"`); n != 3 {
		t.Fatalf("%d model lines, want 3 (an unchanged model writes nothing)", n)
	}
}

func TestOpenSetPerDrawing(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	var ids []string
	for range 3 {
		r, err := st.Create(agentmanager.Codex)
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
		ids = append(ids, r.Meta().ID)
	}
	if _, err := st.LoadOpenFor("a.dxf"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing set: %v", err)
	}
	sets := map[string]store.OpenSet{
		"":                    {IDs: []string{ids[0]}, Active: ids[0]},
		"a.dxf":               {IDs: []string{ids[1], ids[2]}, Active: ids[2]},
		"b with spaces/..dxf": {IDs: []string{ids[2]}},
	}
	for k, o := range sets {
		if err := st.SaveOpenFor(k, o); err != nil {
			t.Fatal(err)
		}
	}
	for k, want := range sets {
		got, err := st.LoadOpenFor(k)
		if err != nil {
			t.Fatal(err)
		}
		if got.Legacy || !reflect.DeepEqual(got.IDs, want.IDs) || got.Active != want.Active {
			t.Errorf("%q: got %+v want %+v", k, got, want)
		}
	}
	if o, _ := st.LoadOpen(); o.Active != ids[0] {
		t.Errorf("LoadOpen is not the set of no drawing: %+v", o)
	}
	if err := st.DeleteOpenFor("a.dxf"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteOpenFor("a.dxf"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := st.LoadOpenFor("a.dxf"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted set: %v", err)
	}
	if _, err := st.LoadOpenFor("b with spaces/..dxf"); err != nil {
		t.Fatalf("other set lost: %v", err)
	}
}

func TestLegacyOpenSet(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	r, _ := st.Create(agentmanager.Claude)
	r.Close()
	id := r.Meta().ID
	if err := os.WriteFile(filepath.Join(dir, "open.json"), []byte(`{"IDs":["`+id+`"],"Active":"`+id+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	o, err := st.LoadOpen()
	if err != nil || !o.Legacy || o.Active != id {
		t.Fatalf("legacy set %+v %v", o, err)
	}
	if err := st.SaveOpen(store.OpenSet{IDs: o.IDs, Active: o.Active, Legacy: true}); err != nil {
		t.Fatal(err)
	}
	if o, _ := st.LoadOpen(); o.Legacy {
		t.Fatal("a saved set reads back as legacy")
	}
}

func TestCopyTo(t *testing.T) {
	src, _ := store.Open(t.TempDir())
	dst, _ := store.Open(filepath.Join(t.TempDir(), "nested", "agent"))
	r, _ := src.Create(agentmanager.OpenCode)
	r.AddUser("draw a door")
	r.Append(agentmanager.TextDelta{Text: "ok"})
	r.Close()
	id := r.Meta().ID
	if err := src.CopyTo(dst, id); err != nil {
		t.Fatal(err)
	}
	got, err := dst.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if m := got.Meta(); m.Title != "draw a door" || m.Harness != "opencode" || len(got.Items()) != 2 {
		t.Fatalf("copied %+v %v", m, got.Items())
	}
	if _, err := src.Load(id); err != nil {
		t.Fatalf("source gone: %v", err)
	}
	if err := src.CopyTo(dst, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing id: %v", err)
	}
	if st, err := os.Stat(filepath.Join(dst.Dir(), id+".jsonl")); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("copied file mode %v %v", st, err)
	}
}
