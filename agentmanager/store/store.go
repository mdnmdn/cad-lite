// Package store persists agent conversations in a directory, one JSONL file per session, so a host
// can show a previous conversation again and resume it with agentmanager.Config.ResumeID. It is
// stdlib only apart from its own module.
package store

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"agentmanager"
	"agentmanager/transcript"
)

// ErrNotFound is returned when a session does not exist or the store is empty.
var ErrNotFound = errors.New("store: session not found")

const (
	fileExt    = ".jsonl"
	titleRunes = 80
)

// Meta describes one stored session.
type Meta struct {
	ID             string
	Harness        string // an agentmanager.Harness value
	AgentSessionID string // backend resume id, "" until the backend reported one
	Title          string // first user prompt, whitespace collapsed, at most 80 runes
	Model          string // the model the user chose (SetModel), "" for the harness default
	Drawing        string // the drawing the host associated (SetDrawing), "" for none
	Created        time.Time
	Updated        time.Time // the file's modification time
}

// Store is a directory of sessions.
type Store struct{ dir string }

// Open returns the store rooted at dir. It does not touch the disk: the directory is created by
// the first Create, and a missing directory lists as empty.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("store: empty directory")
	}
	return &Store{dir: dir}, nil
}

// Dir returns the directory.
func (s *Store) Dir() string { return s.dir }

func (s *Store) path(id string) string { return filepath.Join(s.dir, id+fileExt) }

func validID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\`) && !strings.HasPrefix(id, ".")
}

// List returns the stored sessions, newest first. Unreadable files are skipped.
func (s *Store) List() ([]Meta, error) {
	ents, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Meta
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, fileExt) {
			continue
		}
		m, ok := s.readMeta(strings.TrimSuffix(name, fileExt))
		if ok {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Updated.Equal(out[j].Updated) {
			return out[i].Updated.After(out[j].Updated)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *Store) readMeta(id string) (Meta, bool) {
	f, err := os.Open(s.path(id))
	if err != nil {
		return Meta{}, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Meta{}, false
	}
	m := Meta{ID: id, Updated: st.ModTime()}
	haveMeta := false
	forEachLine(f, func(env envelope) {
		switch env.K {
		case kindMeta:
			var l metaLine
			if json.Unmarshal(env.D, &l) == nil && !haveMeta {
				haveMeta = true
				m.Harness, m.Created = l.Harness, l.Created
			}
		case kindUser:
			if m.Title == "" {
				var u User
				if json.Unmarshal(env.D, &u) == nil {
					m.Title = title(u.Text)
				}
			}
		case kindAgentSess:
			var l agentSessLine
			if json.Unmarshal(env.D, &l) == nil && l.ID != "" {
				m.AgentSessionID = l.ID
			}
		case kindModel, kindDrawing:
			setMetaLine(&m, env)
		case "session_started":
			var e agentmanager.SessionStarted
			if json.Unmarshal(env.D, &e) == nil && e.SessionID != "" {
				m.AgentSessionID = e.SessionID
			}
		}
	})
	return m, haveMeta
}

// setMetaLine applies a model or drawing line to m; the last one wins.
func setMetaLine(m *Meta, env envelope) {
	switch env.K {
	case kindModel:
		var l modelLine
		if json.Unmarshal(env.D, &l) == nil {
			m.Model = l.Model
		}
	case kindDrawing:
		var l drawingLine
		if json.Unmarshal(env.D, &l) == nil {
			m.Drawing = l.Drawing
		}
	}
}

func title(s string) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > titleRunes {
		return string(r[:titleRunes-3]) + "..."
	}
	return string(r)
}

// forEachLine calls f for every parsable line; the rest, including a truncated last line, is skipped.
func forEachLine(r io.Reader, f func(envelope)) (endsWithNewline bool) {
	br := bufio.NewReaderSize(r, 64<<10)
	endsWithNewline = true
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			endsWithNewline = line[len(line)-1] == '\n'
			var env envelope
			if json.Unmarshal(bytes.TrimSpace(line), &env) == nil && env.K != "" {
				f(env)
			}
		}
		if err != nil {
			return endsWithNewline
		}
	}
}

// Create starts a new, empty session for harness. The file is written immediately with its meta line.
func (s *Store) Create(harness agentmanager.Harness) (*Record, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var f *os.File
	var id string
	for range 8 {
		var rnd [4]byte
		if _, err := rand.Read(rnd[:]); err != nil {
			return nil, err
		}
		id = now.Format("20060102T150405.000Z") + "-" + hex.EncodeToString(rnd[:])
		var err error
		f, err = os.OpenFile(s.path(id), os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_APPEND, 0o600)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	if f == nil {
		return nil, errors.New("store: cannot allocate a session id")
	}
	r := &Record{path: s.path(id), f: f, meta: Meta{ID: id, Harness: string(harness), Created: now, Updated: now}}
	line, err := marshalLine(kindMeta, now, metaLine{ID: id, Harness: string(harness), Created: now})
	if err == nil {
		err = r.write(line, true)
	}
	if err != nil {
		r.Close()
		os.Remove(r.path)
		return nil, err
	}
	return r, nil
}

// Load opens the session id with its stored items.
func (s *Store) Load(id string) (*Record, error) {
	if !validID(id) {
		return nil, ErrNotFound
	}
	f, err := os.Open(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := &Record{path: s.path(id), meta: Meta{ID: id}}
	if st, err := f.Stat(); err == nil {
		r.meta.Updated = st.ModTime()
	}
	haveMeta := false
	nl := forEachLine(f, func(env envelope) {
		switch env.K {
		case kindMeta:
			var l metaLine
			if json.Unmarshal(env.D, &l) == nil && !haveMeta {
				haveMeta = true
				r.meta.Harness, r.meta.Created = l.Harness, l.Created
			}
		case kindUser:
			var u User
			if json.Unmarshal(env.D, &u) == nil {
				r.addItem(u)
			}
		case kindResolved:
			var u Resolved
			if json.Unmarshal(env.D, &u) == nil {
				r.addItem(u)
			}
		case kindAgentSess:
			var l agentSessLine
			if json.Unmarshal(env.D, &l) == nil && l.ID != "" {
				r.meta.AgentSessionID = l.ID
			}
		case kindModel, kindDrawing:
			setMetaLine(&r.meta, env)
		default:
			if ev, err := decodeEvent(env); err == nil {
				r.addItem(ev)
				if st, ok := ev.(agentmanager.SessionStarted); ok && st.SessionID != "" {
					r.meta.AgentSessionID = st.SessionID
				}
			}
		}
	})
	if !haveMeta {
		return nil, ErrNotFound
	}
	r.needNL = !nl
	return r, nil
}

// Latest loads the most recently updated session; ErrNotFound when there is none.
func (s *Store) Latest() (*Record, error) {
	ms, err := s.List()
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		return nil, ErrNotFound
	}
	return s.Load(ms[0].ID)
}

// Delete removes the session id; a missing session is not an error.
func (s *Store) Delete(id string) error {
	if !validID(id) {
		return ErrNotFound
	}
	err := os.Remove(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Record is one stored session, open for appending. Its methods are goroutine-safe.
type Record struct {
	mu     sync.Mutex
	path   string
	f      *os.File
	meta   Meta
	items  []any
	needNL bool
}

// Meta returns the session's description as of now.
func (r *Record) Meta() Meta {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.meta
	if m.Title == "" {
		for _, it := range r.items {
			if u, ok := it.(User); ok {
				m.Title = title(u.Text)
				break
			}
		}
	}
	return m
}

func (r *Record) addItem(it any) { r.items = append(r.items, it) }

// Items returns the stored and appended items in order: agentmanager events, User and Resolved.
func (r *Record) Items() []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]any(nil), r.items...)
}

// write appends one line. The file opens on first use; sync forces it to disk.
func (r *Record) write(line []byte, sync bool) error {
	if r.f == nil {
		f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		r.f = f
	}
	buf := make([]byte, 0, len(line)+2)
	if r.needNL {
		buf = append(buf, '\n')
		r.needNL = false
	}
	buf = append(append(buf, line...), '\n')
	if _, err := r.f.Write(buf); err != nil {
		return err
	}
	r.meta.Updated = time.Now()
	if sync {
		return r.f.Sync()
	}
	return nil
}

// Append stores ev. A TurnEnd or Exited also syncs the file; SessionStarted records the resume id.
func (r *Record) Append(ev agentmanager.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	line, err := marshalEvent(ev, time.Now())
	if err != nil {
		return err
	}
	sync := false
	switch ev.(type) {
	case agentmanager.TurnEnd, agentmanager.Exited:
		sync = true
	}
	if err := r.write(line, sync); err != nil {
		return err
	}
	r.items = append(r.items, ev)
	if st, ok := ev.(agentmanager.SessionStarted); ok && st.SessionID != "" {
		r.meta.AgentSessionID = st.SessionID
	}
	return nil
}

// AddUser stores a user message.
func (r *Record) AddUser(text string) error {
	return r.item(kindUser, User{Text: text}, User{Text: text})
}

// ResolvePermission stores that a permission ask was answered.
func (r *Record) ResolvePermission(requestID string) error {
	return r.item(kindResolved, Resolved{RequestID: requestID}, Resolved{RequestID: requestID})
}

func (r *Record) item(kind string, payload, it any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	line, err := marshalLine(kind, time.Now(), payload)
	if err != nil {
		return err
	}
	if err := r.write(line, false); err != nil {
		return err
	}
	r.items = append(r.items, it)
	return nil
}

// SetAgentSessionID stores the backend's resume id; an unchanged id writes nothing.
func (r *Record) SetAgentSessionID(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id == "" || id == r.meta.AgentSessionID {
		return nil
	}
	line, err := marshalLine(kindAgentSess, time.Now(), agentSessLine{ID: id})
	if err != nil {
		return err
	}
	if err := r.write(line, true); err != nil {
		return err
	}
	r.meta.AgentSessionID = id
	return nil
}

// SetModel stores the model the user chose for the session ("" for the harness default); an
// unchanged model writes nothing.
func (r *Record) SetModel(model string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if model == r.meta.Model {
		return nil
	}
	if err := r.metaLine(kindModel, modelLine{Model: model}); err != nil {
		return err
	}
	r.meta.Model = model
	return nil
}

// SetDrawing stores the drawing the host associates the session with ("" for none); an unchanged
// drawing writes nothing.
func (r *Record) SetDrawing(drawing string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if drawing == r.meta.Drawing {
		return nil
	}
	if err := r.metaLine(kindDrawing, drawingLine{Drawing: drawing}); err != nil {
		return err
	}
	r.meta.Drawing = drawing
	return nil
}

func (r *Record) metaLine(kind string, payload any) error {
	line, err := marshalLine(kind, time.Now(), payload)
	if err != nil {
		return err
	}
	return r.write(line, true)
}

// Replay folds the stored items into t. A turn the stored session never finished (the host died
// mid-turn) is closed with a cancelled TurnEnd so the transcript is not stuck running.
func (r *Record) Replay(t *transcript.T) {
	for _, it := range r.Items() {
		switch v := it.(type) {
		case User:
			t.AddUser(v.Text)
		case Resolved:
			t.ResolvePermission(v.RequestID)
		case agentmanager.Event:
			t.Apply(v)
		}
	}
	if t.Running() {
		t.Apply(agentmanager.TurnEnd{Reason: agentmanager.StopCancelled})
	}
}

// Close syncs and closes the file. The record can still be appended to; it reopens the file.
func (r *Record) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Sync()
	if cerr := r.f.Close(); err == nil {
		err = cerr
	}
	r.f = nil
	return err
}

// OpenSet is the sessions a host had open and which one was active.
type OpenSet struct {
	IDs    []string
	Active string
	// Legacy is set by LoadOpen when the file predates per-drawing open sets (no drawing sets
	// existed when it was written); it is never saved.
	Legacy bool `json:"-"`
}

// openFile is the open set of no drawing; a drawing's set is open-<hash>.json.
const (
	openFile    = "open.json"
	openVersion = 2
)

// openDisk is the file format: the set plus the drawing it belongs to and the format version
// (absent in files written before per-drawing sets).
type openDisk struct {
	IDs     []string
	Active  string
	Drawing string `json:",omitempty"`
	V       int    `json:",omitempty"`
}

// openName is the file name of drawing's open set.
func openName(drawing string) string {
	if drawing == "" {
		return openFile
	}
	sum := sha256.Sum256([]byte(drawing))
	return "open-" + hex.EncodeToString(sum[:8]) + ".json"
}

// SaveOpen writes the open set of no drawing; it is SaveOpenFor("", o).
func (s *Store) SaveOpen(o OpenSet) error { return s.SaveOpenFor("", o) }

// LoadOpen reads the open set of no drawing; it is LoadOpenFor("").
func (s *Store) LoadOpen() (OpenSet, error) { return s.LoadOpenFor("") }

// SaveOpenFor writes the open set of drawing atomically. drawing is a host key, such as the
// drawing's file name; "" is the set of no drawing (open.json).
func (s *Store) SaveOpenFor(drawing string, o OpenSet) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	d := openDisk{IDs: o.IDs, Active: o.Active, Drawing: drawing, V: openVersion}
	if d.IDs == nil {
		d.IDs = []string{}
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, openFile+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(b)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(s.dir, openName(drawing)))
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}

// LoadOpenFor reads the open set of drawing; ErrNotFound when none was saved. Ids whose session
// file is gone are dropped, and an Active that is no longer listed is cleared.
func (s *Store) LoadOpenFor(drawing string) (OpenSet, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, openName(drawing)))
	if errors.Is(err, fs.ErrNotExist) {
		return OpenSet{}, ErrNotFound
	}
	if err != nil {
		return OpenSet{}, err
	}
	var d openDisk
	if err := json.Unmarshal(b, &d); err != nil {
		return OpenSet{}, ErrNotFound
	}
	o := OpenSet{Active: d.Active, Legacy: d.V == 0}
	for _, id := range d.IDs {
		if validID(id) {
			if _, err := os.Stat(s.path(id)); err == nil {
				o.IDs = append(o.IDs, id)
			}
		}
	}
	found := false
	for _, id := range o.IDs {
		found = found || id == o.Active
	}
	if !found {
		o.Active = ""
	}
	return o, nil
}

// DeleteOpenFor removes the open set of drawing; a missing one is not an error.
func (s *Store) DeleteOpenFor(drawing string) error {
	err := os.Remove(filepath.Join(s.dir, openName(drawing)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// CopyTo copies session id into dst under the same id, replacing a session of that id there. The
// record of id should be closed (or idle) while it is copied.
func (s *Store) CopyTo(dst *Store, id string) error {
	if !validID(id) {
		return ErrNotFound
	}
	b, err := os.ReadFile(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst.dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(dst.path(id), b, 0o600)
}
