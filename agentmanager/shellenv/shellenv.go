package shellenv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Source says where a probed PATH came from.
type Source string

// The possible sources of a PATH.
const (
	// SourceLoginShell is the PATH printed by the user's login shell.
	SourceLoginShell Source = "login-shell"
	// SourceRegistry is the Machine and User Path read from the Windows registry.
	SourceRegistry Source = "registry"
	// SourceApp means the probe failed and only the app's own PATH is known.
	SourceApp Source = "app"
)

// Probe is the result of Run.
type Probe struct {
	// Path is the PATH the shell or registry reported; empty when Err is set.
	Path string
	// Source is where Path came from; SourceApp when the probe failed.
	Source Source
	// Err is why the probe failed, or nil.
	Err error
}

// Run asks for the user's PATH with LoginPATH and reports where it came from.
func Run(ctx context.Context) Probe {
	p, err := LoginPATH(ctx)
	if err != nil {
		return Probe{Source: SourceApp, Err: err}
	}
	src := SourceLoginShell
	if runtime.GOOS == "windows" {
		src = SourceRegistry
	}
	return Probe{Path: p, Source: src}
}

// Merge returns loginPATH's entries followed by the entries of appPATH that
// loginPATH lacks. Empty entries and repeats are dropped; on Windows entries
// compare without regard to case.
func Merge(appPATH, loginPATH string) string {
	return strings.Join(merge(appPATH, loginPATH, os.PathListSeparator, runtime.GOOS == "windows"), string(os.PathListSeparator))
}

func merge(appPATH, loginPATH string, sep rune, fold bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range []string{loginPATH, appPATH} {
		for _, e := range split(list, sep) {
			k := e
			if fold {
				k = strings.ToLower(k)
			}
			if !seen[k] {
				seen[k] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// Added returns the entries of loginPATH that appPATH lacks, in loginPATH's
// order.
func Added(appPATH, loginPATH string) []string {
	return added(appPATH, loginPATH, os.PathListSeparator, runtime.GOOS == "windows")
}

func added(appPATH, loginPATH string, sep rune, fold bool) []string {
	key := func(s string) string {
		if fold {
			return strings.ToLower(s)
		}
		return s
	}
	have := map[string]bool{}
	for _, e := range split(appPATH, sep) {
		have[key(e)] = true
	}
	var out []string
	for _, e := range split(loginPATH, sep) {
		if !have[key(e)] {
			have[key(e)] = true
			out = append(out, e)
		}
	}
	return out
}

// split cuts a PATH list at sep, trimming blanks and surrounding quotes and
// dropping empty entries.
func split(list string, sep rune) []string {
	var out []string
	for _, e := range strings.Split(list, string(sep)) {
		e = strings.Trim(strings.TrimSpace(e), `"`)
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// Binary is an executable found by Find.
type Binary struct {
	Name string // the name asked for
	Path string // its full path
}

// Find looks each of names up in the directories of path and returns those
// found, in order, with their full paths. On Windows the extensions in
// PATHEXT are tried when a name has none.
func Find(names []string, path string) []Binary {
	var out []Binary
	for _, n := range names {
		if p, ok := Lookup(n, path); ok {
			out = append(out, Binary{Name: n, Path: p})
		}
	}
	return out
}

// Lookup returns the full path of the executable name in the directories of
// path.
func Lookup(name, path string) (string, bool) {
	return lookup(name, path, runtime.GOOS, os.Getenv("PATHEXT"))
}

func lookup(name, path, goos, pathext string) (string, bool) {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return "", false
	}
	win := goos == "windows"
	sep := ':'
	names := []string{name}
	if win {
		sep = ';'
		if pathext == "" {
			pathext = ".COM;.EXE;.BAT;.CMD"
		}
		if filepath.Ext(name) == "" {
			names = names[:0]
			for _, e := range split(pathext, ';') {
				names = append(names, name+strings.ToLower(e))
			}
		}
	}
	for _, dir := range split(path, sep) {
		for _, n := range names {
			p := filepath.Join(dir, n)
			if executable(p, win) {
				return p, true
			}
		}
	}
	return "", false
}

func executable(p string, win bool) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	return win || fi.Mode().Perm()&0o111 != 0
}

var state struct {
	sync.Mutex
	login string
}

// Set stores the PATH of the user's shell for Effective and Env. Call it
// with the result of LoginPATH; an empty string forgets it.
func Set(loginPATH string) {
	state.Lock()
	state.login = loginPATH
	state.Unlock()
}

// Effective is the process's PATH merged with the one stored by Set.
func Effective() string {
	state.Lock()
	login := state.login
	state.Unlock()
	return Merge(os.Getenv("PATH"), login)
}

// Env returns the environment entry that gives a child process the
// effective PATH, or nil while no login PATH is known.
func Env() []string {
	state.Lock()
	login := state.login
	state.Unlock()
	if login == "" {
		return nil
	}
	return []string{"PATH=" + Merge(os.Getenv("PATH"), login)}
}

// Resolve returns the full path of the executable name on the effective PATH.
func Resolve(name string) (string, bool) {
	if p, ok := Lookup(name, Effective()); ok {
		return p, true
	}
	p, err := exec.LookPath(name)
	return p, err == nil
}
