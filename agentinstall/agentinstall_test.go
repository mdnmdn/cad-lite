package agentinstall

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeMise is a shell script standing in for mise: `install` writes a fake opencode (whose
// `auth list` prints $FAKE_AUTH) under MISE_DATA_DIR and records its environment and working
// directory under MISE_STATE_DIR; `which` prints that opencode's path.
const fakeMise = `#!/bin/sh
case "$1" in
--version) echo "mise 2026.1.0-fake"; exit 0 ;;
install)
  if [ "$FAKE_MISE_SLEEP" = 1 ]; then exec sleep 30; fi
  if [ "$FAKE_MISE_FAIL" = 1 ]; then echo "boom: registry unreachable" >&2; exit 1; fi
  mkdir -p "$MISE_STATE_DIR"
  env | grep '^MISE_' | sort > "$MISE_STATE_DIR/env.txt"
  pwd > "$MISE_STATE_DIR/pwd.txt"
  echo "$2" > "$MISE_STATE_DIR/tool.txt"
  d="$MISE_DATA_DIR/installs/opencode/1.2.3/bin"
  mkdir -p "$d"
  cat > "$d/opencode" <<'EOS'
#!/bin/sh
if [ "$1 $2" = "auth list" ]; then printf '%b' "$FAKE_AUTH"; exit 0; fi
echo "opencode 1.2.3"
EOS
  chmod +x "$d/opencode"
  echo "installed opencode" >&2
  ;;
which) echo "$MISE_DATA_DIR/installs/opencode/1.2.3/bin/opencode" ;;
cache) exit 0 ;;
*) exit 2 ;;
esac
`

const testVersion = "2026.1.0"

// testAsset is the archive name the tests' installer (linux/amd64) asks for.
var testAsset = "mise-v" + testVersion + "-linux-x64-musl.tar.gz"

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// release is a fake mise releases page.
type release struct {
	archive []byte
	sums    string
	// stall, when set, makes the archive handler send half of it and wait for the client to go.
	stall     bool
	downloads atomic.Int32
}

func newRelease(t *testing.T) *release {
	t.Helper()
	a := tarGz(t, map[string]string{"mise/bin/mise": fakeMise, "mise/README.md": "fake"})
	return &release{archive: a, sums: sum(a) + "  ./" + testAsset + "\n" +
		strings.Repeat("0", 64) + "  ./mise-v" + testVersion + "-macos-arm64.tar.gz\n" +
		strings.Repeat("1", 64) + "  ./mise-v" + testVersion + "-linux-x64\n"}
}

func (r *release) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/latest/download/" + ChecksumFile:
			w.Write([]byte(r.sums))
		case "/download/v" + testVersion + "/" + testAsset:
			r.downloads.Add(1)
			w.Header().Set("Content-Length", strconv.Itoa(len(r.archive)))
			if r.stall {
				w.Write(r.archive[:len(r.archive)/2])
				w.(http.Flusher).Flush()
				<-req.Context().Done()
				return
			}
			w.Write(r.archive)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// progressLog collects Progress reports from the installing goroutine.
type progressLog struct {
	mu sync.Mutex
	l  []Progress
}

func (p *progressLog) add(x Progress) { p.mu.Lock(); p.l = append(p.l, x); p.mu.Unlock() }

func (p *progressLog) all() []Progress {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Progress(nil), p.l...)
}

func newInstaller(t *testing.T, srv *httptest.Server, env ...string) (*Installer, *progressLog) {
	t.Helper()
	if runtime.GOOS == "windows" || runtime.GOOS == "js" {
		t.Skip("the fake mise is a shell script")
	}
	log := &progressLog{}
	return &Installer{Dir: t.TempDir(), BaseURL: srv.URL, GOOS: "linux", GOARCH: "amd64", Env: env, Progress: log.add}, log
}

const framedNone = "┌  Credentials ~/.local/share/opencode/auth.json\n│\n└  0 credentials\n\n┌  Environment\n│\n└  0 environment variables\n"

func TestInstallDownloadsMiseInstallsOpenCodeAndChecksAuth(t *testing.T) {
	rel := newRelease(t)
	srv := rel.serve(t)
	in, log := newInstaller(t, srv, "FAKE_AUTH="+framedNone)
	r, err := in.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Mise != filepath.Join(in.Dir, "mise", "bin", "mise") {
		t.Errorf("mise at %s", r.Mise)
	}
	if want := filepath.Join(in.Dir, "mise", "data", "installs", "opencode", "1.2.3", "bin", "opencode"); r.Binary != want {
		t.Errorf("binary %s, want %s", r.Binary, want)
	}
	if r.Authenticated || r.AuthErr != nil {
		t.Errorf("no credentials were listed: %+v", r)
	}
	if fi, err := os.Stat(r.Mise); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("mise is not executable: %v %v", fi, err)
	}

	state := filepath.Join(in.Dir, "mise", "state")
	env, _ := os.ReadFile(filepath.Join(state, "env.txt"))
	for _, want := range []string{
		"MISE_DATA_DIR=" + filepath.Join(in.Dir, "mise", "data"),
		"MISE_CONFIG_DIR=" + filepath.Join(in.Dir, "mise", "config"),
		"MISE_CACHE_DIR=" + filepath.Join(in.Dir, "mise", "cache"),
		"MISE_STATE_DIR=" + state,
		"MISE_GLOBAL_CONFIG_FILE=" + filepath.Join(in.Dir, "mise", "config", "config.toml"),
		"MISE_CEILING_PATHS=" + in.Dir,
		"MISE_YES=1",
	} {
		if !strings.Contains(string(env), want+"\n") {
			t.Errorf("mise ran without %s:\n%s", want, env)
		}
	}
	pwd, _ := os.ReadFile(filepath.Join(state, "pwd.txt"))
	if got, _ := filepath.EvalSymlinks(strings.TrimSpace(string(pwd))); got != mustEval(t, filepath.Join(in.Dir, "work")) {
		t.Errorf("mise ran in %q", pwd)
	}
	if tool, _ := os.ReadFile(filepath.Join(state, "tool.txt")); strings.TrimSpace(string(tool)) != DefaultTool {
		t.Errorf("mise installed %q", tool)
	}

	var steps []Step
	var lastDone int64
	for _, p := range log.all() {
		if len(steps) == 0 || steps[len(steps)-1] != p.Step {
			steps = append(steps, p.Step)
		}
		if p.Step == StepMise && p.Done > 0 {
			lastDone = p.Done
			if p.Total != int64(len(rel.archive)) {
				t.Errorf("total %d, want %d", p.Total, len(rel.archive))
			}
		}
	}
	if len(steps) != 3 || steps[0] != StepMise || steps[1] != StepTool || steps[2] != StepAuth {
		t.Errorf("steps %v", steps)
	}
	if lastDone != int64(len(rel.archive)) {
		t.Errorf("progress ended at %d of %d", lastDone, len(rel.archive))
	}
	if left, _ := filepath.Glob(filepath.Join(in.Dir, "mise", ".download-*")); len(left) != 0 {
		t.Errorf("download folder left behind: %v", left)
	}

	// A second run reuses the private mise.
	if _, err := in.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := rel.downloads.Load(); n != 1 {
		t.Errorf("mise downloaded %d times", n)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestInstallReportsCredentials(t *testing.T) {
	srv := newRelease(t).serve(t)
	in, _ := newInstaller(t, srv, `FAKE_AUTH=Anthropic  oauth  ~/.local/share/opencode/auth.json\n`)
	r, err := in.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Authenticated {
		t.Errorf("a stored credential was not seen: %+v", r)
	}
}

func TestChecksumMismatchIsRefused(t *testing.T) {
	rel := newRelease(t)
	rel.sums = strings.Repeat("a", 64) + "  ./" + testAsset + "\n"
	in, _ := newInstaller(t, rel.serve(t))
	_, err := in.Install(context.Background())
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v, want ErrChecksum", err)
	}
	assertNoMise(t, in)
}

func TestMissingChecksumIsRefused(t *testing.T) {
	rel := newRelease(t)
	rel.sums = strings.Repeat("a", 64) + "  ./mise-v" + testVersion + "-macos-arm64.tar.gz\n"
	in, _ := newInstaller(t, rel.serve(t))
	if _, err := in.Install(context.Background()); !errors.Is(err, ErrNoChecksum) {
		t.Fatalf("err = %v, want ErrNoChecksum", err)
	}
	if n := rel.downloads.Load(); n != 0 {
		t.Errorf("an unlisted archive was downloaded")
	}
	assertNoMise(t, in)
}

func assertNoMise(t *testing.T, in *Installer) {
	t.Helper()
	if _, err := os.Stat(in.misePath()); err == nil {
		t.Error("an unverified mise was installed")
	}
	if left, _ := filepath.Glob(filepath.Join(in.Dir, "mise", ".download-*")); len(left) != 0 {
		t.Errorf("download folder left behind: %v", left)
	}
}

func TestCancelDuringDownload(t *testing.T) {
	rel := newRelease(t)
	rel.stall = true
	srv := rel.serve(t)
	in, _ := newInstaller(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	in.Progress = func(p Progress) {
		if p.Step == StepMise && p.Done > 0 {
			once.Do(cancel)
		}
	}
	done := make(chan error, 1)
	go func() { _, err := in.Install(ctx); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancel did not stop the download")
	}
	assertNoMise(t, in)
}

func TestCancelDuringToolInstall(t *testing.T) {
	srv := newRelease(t).serve(t)
	in, _ := newInstaller(t, srv, "FAKE_MISE_SLEEP=1")
	ctx, cancel := context.WithCancel(context.Background())
	in.Progress = func(p Progress) {
		if p.Step == StepTool {
			go func() { time.Sleep(100 * time.Millisecond); cancel() }()
		}
	}
	start := time.Now()
	_, err := in.Install(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("cancel took %v", d)
	}
}

func TestToolFailureCarriesItsOutput(t *testing.T) {
	srv := newRelease(t).serve(t)
	in, _ := newInstaller(t, srv, "FAKE_MISE_FAIL=1")
	_, err := in.Install(context.Background())
	if err == nil || !strings.Contains(err.Error(), "registry unreachable") {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallRefusesARelativeDir(t *testing.T) {
	in := &Installer{Dir: "tools"}
	if _, err := in.Install(context.Background()); err == nil {
		t.Fatal("a relative tools folder was accepted")
	}
	if _, err := (&Installer{}).Install(context.Background()); err == nil {
		t.Fatal("an empty tools folder was accepted")
	}
}

func TestCheckURLRefusesPlainHTTP(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://github.com/jdx/mise": true,
		"http://127.0.0.1:8080/x":     true,
		"http://localhost/x":          true,
		"http://github.com/jdx/mise":  false,
		"ftp://example.com/mise":      false,
	} {
		u, _ := url.Parse(raw)
		if err := checkURL(u); (err == nil) != ok {
			t.Errorf("checkURL(%s) = %v", raw, err)
		}
	}
}

func TestHasCredentials(t *testing.T) {
	for _, c := range []struct {
		name, out string
		want      bool
	}{
		{"empty", "", false},
		{"framed none", framedNone, false},
		{"framed one", "┌  Credentials ~/.local/share/opencode/auth.json\n│\n●  Anthropic oauth\n│\n└  1 credentials\n", true},
		{"framed env only", "┌  Credentials x\n└  0 credentials\n┌  Environment\n●  OpenAI OPENAI_API_KEY\n└  1 environment variables\n", true},
		{"coloured", "\x1b[90m┌\x1b[0m  Credentials\n\x1b[90m└\x1b[0m  \x1b[1m0\x1b[0m credentials\n", false},
		{"plain one", "Anthropic        oauth        ~/.local/share/opencode/auth.json\n", true},
		{"plain env", "GitHub Copilot  GITHUB_TOKEN                environment\n", true},
		{"plain none", "No credentials found. Run `opencode auth login`.\n", false},
		{"not logged in", "You are not logged in to any provider\n", false},
	} {
		if got := HasCredentials(c.out); got != c.want {
			t.Errorf("%s: HasCredentials = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAssetSuffix(t *testing.T) {
	for _, c := range []struct{ goos, goarch, want string }{
		{"linux", "amd64", "-linux-x64-musl.tar.gz"},
		{"linux", "arm64", "-linux-arm64-musl.tar.gz"},
		{"linux", "arm", "-linux-armv7-musl.tar.gz"},
		{"darwin", "arm64", "-macos-arm64.tar.gz"},
		{"darwin", "amd64", "-macos-x64.tar.gz"},
		{"windows", "amd64", "-windows-x64.zip"},
		{"windows", "arm64", "-windows-arm64.zip"},
		{"linux", "386", ""},
		{"js", "wasm", ""},
		{"freebsd", "amd64", ""},
	} {
		got, ok := AssetSuffix(c.goos, c.goarch)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("AssetSuffix(%s, %s) = %q, %v", c.goos, c.goarch, got, ok)
		}
	}
}

func TestPickAssetTakesTheArchiveNotTheBareBinary(t *testing.T) {
	sums := map[string]string{
		"./mise-v2026.10.10-linux-x64":                 "a",
		"./mise-v2026.10.10-linux-x64-musl":            "b",
		"./mise-v2026.10.10-linux-x64-musl.tar.gz":     "c",
		"./mise-v2026.10.10-linux-x64-musl.tar.zst":    "d",
		"./mise-v2026.10.10-linux-arm64-musl.tar.gz":   "e",
		"./mise-v2026.10.10-windows-x64.zip":           "f",
		"./sub/mise-v2026.10.10-linux-x64-musl.tar.gz": "g",
	}
	name, ver, s, ok := pickAsset(sums, "-linux-x64-musl.tar.gz")
	if !ok || name != "mise-v2026.10.10-linux-x64-musl.tar.gz" || ver != "2026.10.10" || s != "c" {
		t.Fatalf("pickAsset = %q %q %q %v", name, ver, s, ok)
	}
	if _, _, _, ok := pickAsset(sums, "-macos-arm64.tar.gz"); ok {
		t.Fatal("found an archive that is not listed")
	}
}

func TestDefaultDir(t *testing.T) {
	home := func() (string, error) { return "/home/u", nil }
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for _, c := range []struct {
		goos string
		env  map[string]string
		want string
	}{
		{"linux", nil, filepath.Join("/home/u", ".local", "share", "cad-lite", "tools")},
		{"linux", map[string]string{"XDG_DATA_HOME": "/data"}, filepath.Join("/data", "cad-lite", "tools")},
		{"linux", map[string]string{"XDG_DATA_HOME": "relative"}, filepath.Join("/home/u", ".local", "share", "cad-lite", "tools")},
		{"darwin", nil, filepath.Join("/home/u", "Library", "Application Support", "cad-lite", "tools")},
		{"windows", map[string]string{"LocalAppData": `C:\Users\u\AppData\Local`}, filepath.Join(`C:\Users\u\AppData\Local`, "cad-lite", "tools")},
	} {
		got, err := defaultDir(c.goos, env(c.env), home)
		if err != nil || got != c.want {
			t.Errorf("%s %v: %q, %v; want %q", c.goos, c.env, got, err, c.want)
		}
	}
	if _, err := defaultDir("windows", env(nil), home); err == nil {
		t.Error("windows without LocalAppData gave a folder")
	}
	if _, err := defaultDir("js", env(nil), home); err == nil {
		t.Error("js gave a folder")
	}
}

func TestDirHonoursTheEnvironment(t *testing.T) {
	t.Setenv("CADLITE_"+EnvDir, "/opt/cad-tools")
	if d, err := Dir(); err != nil || d != "/opt/cad-tools" {
		t.Fatalf("Dir() = %q, %v", d, err)
	}
}

func TestLoginArgs(t *testing.T) {
	got := LoginArgs("/x/opencode")
	if strings.Join(got, " ") != "/x/opencode auth login" {
		t.Fatalf("LoginArgs = %v", got)
	}
}
