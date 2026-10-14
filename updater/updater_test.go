package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.4", "1.2.3", 1},
		{"1.10.0", "1.9.9", 1},
		{"v2.0.0", "1.99.99", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.0.0+abc", "1.0.0+def", 0},
		{"0.0.0-ci+abc", "0.1.0", -1},
	}
	for _, c := range cases {
		a, err := ParseVersion(c.a)
		if err != nil {
			t.Fatal(c.a, err)
		}
		b, err := ParseVersion(c.b)
		if err != nil {
			t.Fatal(c.b, err)
		}
		if got := a.Compare(b); got != c.want {
			t.Errorf("%s vs %s: got %d want %d", c.a, c.b, got, c.want)
		}
	}
	for _, bad := range []string{"", "dev", "1.2", "1.2.3.4", "01.2.3", "1.2.x", "1.2.3-", "1.2.3+"} {
		if _, err := ParseVersion(bad); err == nil {
			t.Errorf("ParseVersion(%q) succeeded", bad)
		}
	}
	for cur, want := range map[string]bool{"1.2.3": true, "v1.2.3": true, "1.0.0-rc.1": true, "dev": false, "": false, "0.0.0-ci+abc1234": false} {
		if Updatable(cur) != want {
			t.Errorf("Updatable(%q) = %v", cur, !want)
		}
	}
}

func TestAssetSelection(t *testing.T) {
	rel := Release{Version: "1.2.3"}
	for _, n := range []string{
		"cad-lite-1.2.3-macos-universal.zip", "cad-lite-1.2.3-macos-universal.dmg",
		"cad-lite-1.2.3-windows-amd64.zip", "cad-lite-1.2.3-windows-amd64-setup.exe", "cad-lite-1.2.3-windows-arm64.zip",
		"cad-lite-1.2.3-linux-amd64.tar.gz", "checksums.txt",
	} {
		rel.Assets = append(rel.Assets, Asset{Name: n})
	}
	for _, c := range []struct{ goos, arch, want string }{
		{"darwin", "arm64", "cad-lite-1.2.3-macos-universal.zip"},
		{"darwin", "amd64", "cad-lite-1.2.3-macos-universal.zip"},
		{"windows", "amd64", "cad-lite-1.2.3-windows-amd64.zip"},
		{"windows", "arm64", "cad-lite-1.2.3-windows-arm64.zip"},
		{"linux", "amd64", "cad-lite-1.2.3-linux-amd64.tar.gz"},
		{"linux", "arm64", ""},
		{"freebsd", "amd64", ""},
	} {
		a, ok := rel.AssetFor(c.goos, c.arch)
		if a.Name != c.want || ok != (c.want != "") {
			t.Errorf("%s/%s: got %q %v, want %q", c.goos, c.arch, a.Name, ok, c.want)
		}
	}
	if a, ok := rel.ChecksumAsset(); !ok || a.Name != "checksums.txt" {
		t.Errorf("checksum asset = %v %v", a, ok)
	}
}

// fakeGitHub serves a release with the given tag and assets.
type fakeGitHub struct {
	srv      *httptest.Server
	tag      string
	draft    bool
	pre      bool
	assets   map[string][]byte
	sums     string // checksums.txt body override; empty builds it from assets
	hits     atomic.Int32
	notMod   atomic.Int32
	etagSeen atomic.Value
}

func newFakeGitHub(t *testing.T, tag string, assets map[string][]byte) *fakeGitHub {
	f := &fakeGitHub{tag: tag, assets: assets}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/mdnmdn/go-draw/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "no UA", 403)
			return
		}
		f.etagSeen.Store(r.Header.Get("If-None-Match"))
		if r.Header.Get("If-None-Match") == `"v1"` {
			f.notMod.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		type asset struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		}
		out := map[string]any{"tag_name": f.tag, "name": "Release " + f.tag, "body": "notes", "html_url": f.srv.URL + "/rel",
			"draft": f.draft, "prerelease": f.pre}
		var as []asset
		for n, b := range f.assets {
			as = append(as, asset{n, f.srv.URL + "/dl/" + n, int64(len(b))})
		}
		as = append(as, asset{"checksums.txt", f.srv.URL + "/dl/checksums.txt", 0})
		out["assets"] = as
		w.Header().Set("ETag", `"v1"`)
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/dl/")
		if name == "checksums.txt" {
			if f.sums != "" {
				w.Write([]byte(f.sums))
				return
			}
			for n, b := range f.assets {
				fmt.Fprintf(w, "%x  %s\n", sha256.Sum256(b), n)
			}
			return
		}
		b, ok := f.assets[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) client(goos, arch string) *Client {
	c := NewClient("test")
	c.BaseURL, c.GOOS, c.GOARCH = f.srv.URL, goos, arch
	return c
}

func skipJS(t *testing.T) {
	if !Supported() {
		t.Skip("unsupported platform")
	}
}

func TestCheckVersions(t *testing.T) {
	skipJS(t)
	assets := map[string][]byte{"cad-lite-1.5.0-linux-amd64.tar.gz": []byte("x")}
	f := newFakeGitHub(t, "v1.5.0", assets)
	for _, c := range []struct {
		cur       string
		available bool
	}{
		{"1.4.9", true}, {"1.5.0", false}, {"1.5.1", false}, {"1.5.0-rc.1", true},
		{"dev", false}, {"0.0.0-ci+abc1234", false},
	} {
		cl := f.client("linux", "amd64")
		rel, ok, err := cl.Check(context.Background(), c.cur)
		if err != nil || ok != c.available {
			t.Errorf("current %q: ok=%v err=%v, want %v", c.cur, ok, err, c.available)
		}
		if rel.Version != "1.5.0" {
			t.Errorf("current %q: latest %q, want 1.5.0 even when not newer", c.cur, rel.Version)
		}
	}
}

func TestCheckETag(t *testing.T) {
	skipJS(t)
	f := newFakeGitHub(t, "v1.5.0", map[string][]byte{"cad-lite-1.5.0-linux-amd64.tar.gz": []byte("x")})
	c := f.client("linux", "amd64")
	if _, ok, err := c.Check(context.Background(), "1.0.0"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	rel, ok, err := c.Check(context.Background(), "1.0.0")
	if err != nil || !ok || rel.Version != "1.5.0" {
		t.Fatalf("second check: %v %v %v", rel.Version, ok, err)
	}
	if f.notMod.Load() != 1 {
		t.Errorf("want one 304, got %d", f.notMod.Load())
	}
	if _, ok, err := c.CheckForce(context.Background(), "1.0.0"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if f.notMod.Load() != 1 || f.etagSeen.Load() != "" {
		t.Errorf("CheckForce sent If-None-Match %q", f.etagSeen.Load())
	}
}

func TestCheckSkipsDraftsPrereleasesAndMissingAsset(t *testing.T) {
	skipJS(t)
	f := newFakeGitHub(t, "v2.0.0", map[string][]byte{"cad-lite-2.0.0-linux-amd64.tar.gz": nil})
	f.draft = true
	rel, ok, err := f.client("linux", "amd64").Check(context.Background(), "1.0.0")
	if err != nil || ok || !rel.IsZero() {
		t.Errorf("draft: %v %v %v", rel.Tag, ok, err)
	}
	f.draft, f.pre = false, true
	if rel, ok, _ := f.client("linux", "amd64").Check(context.Background(), "1.0.0"); ok || !rel.IsZero() {
		t.Errorf("prerelease offered: %v", rel.Tag)
	}
	f.pre = false
	rel, ok, err = f.client("windows", "amd64").Check(context.Background(), "1.0.0")
	if ok || !errors.Is(err, ErrNoAsset) || rel.Tag != "v2.0.0" {
		t.Errorf("missing asset: %v %v %v", rel.Tag, ok, err)
	}
}

func TestCheckErrors(t *testing.T) {
	skipJS(t)
	for status, want := range map[int]string{404: "", 403: "rate limit", 500: "500"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		c := NewClient("t")
		c.BaseURL = srv.URL
		_, ok, err := c.Check(context.Background(), "1.0.0")
		srv.Close()
		if ok || (want == "") != (err == nil) || (want != "" && !strings.Contains(err.Error(), want)) {
			t.Errorf("status %d: ok=%v err=%v", status, ok, err)
		}
	}
	c := NewClient("t")
	c.BaseURL = "http://example.com"
	if _, _, err := c.Check(context.Background(), "1.0.0"); err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Errorf("plain http to a remote host: %v", err)
	}
}

func zipBytes(t *testing.T, files map[string]string, modes map[string]os.FileMode) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for n, body := range files {
		h := &zip.FileHeader{Name: n, Method: zip.Deflate}
		m := os.FileMode(0o644)
		if x, ok := modes[n]; ok {
			m = x
		}
		h.SetMode(m)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func tgzBytes(t *testing.T, files map[string]string) []byte {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for n, body := range files {
		mode := int64(0o644)
		if strings.Contains(n, "bin/") {
			mode = 0o755
		}
		tw.WriteHeader(&tar.Header{Name: n, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return b.Bytes()
}

func writeTemp(t *testing.T, name string, data []byte) string {
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractZipAndTarGz(t *testing.T) {
	dest := t.TempDir()
	z := writeTemp(t, "a.zip", zipBytes(t, map[string]string{"dir/cad-lite.exe": "EXE", "dir/readme.txt": "hi"}, map[string]os.FileMode{"dir/cad-lite.exe": 0o755}))
	if err := Extract(z, dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "dir", "cad-lite.exe")); string(b) != "EXE" {
		t.Errorf("zip content %q", b)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(filepath.Join(dest, "dir", "cad-lite.exe")); st.Mode().Perm() != 0o755 {
			t.Errorf("zip mode %v", st.Mode())
		}
	}
	dest = t.TempDir()
	g := writeTemp(t, "a.tar.gz", tgzBytes(t, map[string]string{"cad-lite-1-linux-amd64/bin/cad-lite": "BIN"}))
	if err := Extract(g, dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "cad-lite-1-linux-amd64", "bin", "cad-lite")); string(b) != "BIN" {
		t.Errorf("tar content %q", b)
	}
	if err := Extract(writeTemp(t, "a.rar", nil), dest); err == nil {
		t.Error("unknown archive type accepted")
	}
}

// Windows PowerShell's Compress-Archive writes backslashes in entry names (the v0.1.0
// Windows zips have them); they must unpack into folders on every OS.
func TestExtractZipWithBackslashNames(t *testing.T) {
	dest := t.TempDir()
	z := writeTemp(t, "w.zip", zipBytes(t, map[string]string{`cad-lite-windows-amd64\cad-lite.exe`: "EXE"}, nil))
	if err := Extract(z, dest); err != nil {
		t.Fatal(err)
	}
	if p, ok := findFile(dest, "cad-lite.exe"); !ok || filepath.Dir(p) != filepath.Join(dest, "cad-lite-windows-amd64") {
		t.Fatalf("cad-lite.exe not unpacked into its folder: %q %v", p, ok)
	}
}

func TestExtractRejectsEscapes(t *testing.T) {
	for _, name := range []string{"../evil", "a/../../evil", "/abs/evil", `..\evil`, `a\..\..\evil`} {
		dest := t.TempDir()
		z := writeTemp(t, "z.zip", zipBytes(t, map[string]string{name: "x"}, nil))
		if err := Extract(z, dest); err == nil {
			t.Errorf("zip entry %q accepted", name)
		}
		g := writeTemp(t, "g.tar.gz", tgzBytes(t, map[string]string{name: "x"}))
		if err := Extract(g, dest); err == nil {
			t.Errorf("tar entry %q accepted", name)
		}
	}
}

func TestDownloadVerifies(t *testing.T) {
	skipJS(t)
	body := []byte("archive-bytes")
	name := "cad-lite-1.5.0-linux-amd64.tar.gz"
	f := newFakeGitHub(t, "v1.5.0", map[string][]byte{name: body})
	c := f.client("linux", "amd64")
	rel, _, err := c.Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	var last [2]int64
	p, err := c.Download(context.Background(), rel, t.TempDir(), func(d, tot int64) { last = [2]int64{d, tot} })
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); !bytes.Equal(got, body) {
		t.Errorf("downloaded %q", got)
	}
	if last[0] != int64(len(body)) || last[1] != int64(len(body)) {
		t.Errorf("progress ended at %v", last)
	}

	// Mismatch: the list names a different hash.
	f.sums = strings.Repeat("0", 64) + "  " + name + "\n"
	dir := t.TempDir()
	if _, err := c.Download(context.Background(), rel, dir, nil); !errors.Is(err, ErrChecksum) {
		t.Errorf("mismatch: %v", err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("unverified file left behind: %v", ents)
	}
	// Not listed at all.
	f.sums = strings.Repeat("0", 64) + "  other.zip\n"
	if _, err := c.Download(context.Background(), rel, t.TempDir(), nil); !errors.Is(err, ErrNoChecksum) {
		t.Errorf("unlisted: %v", err)
	}
	// No checksums asset.
	rel2 := rel
	rel2.Assets = nil
	for _, a := range rel.Assets {
		if a.Name != "checksums.txt" {
			rel2.Assets = append(rel2.Assets, a)
		}
	}
	if _, err := c.Download(context.Background(), rel2, t.TempDir(), nil); !errors.Is(err, ErrNoChecksum) {
		t.Errorf("no list: %v", err)
	}
	// Malformed list.
	f.sums = "garbage\n"
	if _, err := c.Download(context.Background(), rel, t.TempDir(), nil); err == nil {
		t.Error("malformed list accepted")
	}
}

func TestParseChecksums(t *testing.T) {
	h := strings.Repeat("ab", 32)
	m, err := ParseChecksums(strings.NewReader(h + "  a.zip\n" + strings.ToUpper(h) + " *b.zip\n\n# c\n"))
	if err != nil || m["a.zip"] != h || m["b.zip"] != h {
		t.Errorf("%v %v", m, err)
	}
}

func TestInstallWindows(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "cad-lite.exe")
	os.WriteFile(exe, []byte("OLD"), 0o755)
	z := writeTemp(t, "cad-lite-1.5.0-windows-amd64.zip", zipBytes(t, map[string]string{
		"cad-lite-windows-amd64/cad-lite.exe": "NEW"}, nil))
	plan, err := Install(z, "windows", exe)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" {
		t.Errorf("exe = %q", b)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "OLD" {
		t.Errorf(".old = %q (kept for the next start)", b)
	}
	if plan.Exe != exe || plan.App != "" {
		t.Errorf("plan %+v", plan)
	}
	ents, _ := os.ReadDir(root)
	if len(ents) != 2 {
		t.Errorf("staging left behind: %v", ents)
	}
	// A second update replaces a stale .old.
	if _, err := Install(z, "windows", exe); err != nil {
		t.Fatal(err)
	}
}

func TestInstallLinux(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "bin", "cad-lite")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("OLD"), 0o755)
	g := writeTemp(t, "x.tar.gz", tgzBytes(t, map[string]string{
		"cad-lite-1.5.0-linux-amd64/bin/cad-lite": "NEW",
		"cad-lite-1.5.0-linux-amd64/install.sh":   "#!/bin/sh",
		"cad-lite-1.5.0-linux-amd64/share/readme": "r",
	}))
	if _, err := Install(g, "linux", exe); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" {
		t.Errorf("exe = %q", b)
	}
	if _, err := os.Stat(exe + ".old"); err == nil {
		t.Error(".old not removed on linux")
	}
	if st, _ := os.Stat(exe); st.Mode().Perm()&0o111 == 0 {
		t.Errorf("not executable: %v", st.Mode())
	}
	if ents, _ := os.ReadDir(filepath.Dir(exe)); len(ents) != 1 {
		t.Errorf("leftovers: %v", ents)
	}
}

func macZip(t *testing.T, content string) []byte {
	return zipBytes(t, map[string]string{
		"Cad-lite.app/Contents/MacOS/cad-lite": content,
		"Cad-lite.app/Contents/Info.plist":     "<plist>" + content + "</plist>",
	}, map[string]os.FileMode{"Cad-lite.app/Contents/MacOS/cad-lite": 0o755})
}

func TestInstallMacBundle(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "Apps", "Cad-lite.app")
	exe := filepath.Join(app, "Contents", "MacOS", "cad-lite")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("OLD"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "stale"), []byte("s"), 0o644)
	z := writeTemp(t, "m.zip", macZip(t, "NEW"))
	plan, err := Install(z, "darwin", exe)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" {
		t.Errorf("exe = %q", b)
	}
	if _, err := os.Stat(filepath.Join(app, "Contents", "stale")); err == nil {
		t.Error("the bundle was merged, not replaced")
	}
	if plan.App != app {
		t.Errorf("plan.App = %q", plan.App)
	}
	ents, _ := os.ReadDir(filepath.Dir(app))
	if len(ents) != 1 {
		t.Errorf("leftovers beside the bundle: %v", ents)
	}
}

func TestInstallMacKeepsOldWhenBundleIsBad(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "Cad-lite.app")
	exe := filepath.Join(app, "Contents", "MacOS", "cad-lite")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("OLD"), 0o755)
	z := writeTemp(t, "m.zip", zipBytes(t, map[string]string{"Cad-lite.app/Contents/Info.plist": "x"}, nil))
	if _, err := Install(z, "darwin", exe); err == nil {
		t.Fatal("bundle without executable accepted")
	}
	if b, _ := os.ReadFile(exe); string(b) != "OLD" {
		t.Errorf("old version damaged: %q", b)
	}
	if ents, _ := os.ReadDir(root); len(ents) != 1 {
		t.Errorf("leftovers: %v", ents)
	}
}

func TestInstallMacBareBinary(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "cad-lite")
	os.WriteFile(exe, []byte("OLD"), 0o755)
	z := writeTemp(t, "m.zip", macZip(t, "NEW"))
	plan, err := Install(z, "darwin", exe)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" || plan.App != "" {
		t.Errorf("exe = %q plan = %+v", b, plan)
	}
}

func TestInstallNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions are not enforced here")
	}
	root := t.TempDir()
	exe := filepath.Join(root, "cad-lite")
	os.WriteFile(exe, []byte("OLD"), 0o755)
	os.Chmod(root, 0o555)
	defer os.Chmod(root, 0o755)
	g := writeTemp(t, "x.tar.gz", tgzBytes(t, map[string]string{"d/bin/cad-lite": "NEW"}))
	if _, err := Install(g, "linux", exe); !errors.Is(err, ErrNotWritable) {
		t.Errorf("err = %v", err)
	}
}

func TestInstallSwapRollsBack(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "t")
	os.WriteFile(target, []byte("OLD"), 0o755)
	if err := swap(target, filepath.Join(root, "missing"), true); err == nil {
		t.Fatal("swap with a missing source succeeded")
	}
	if b, _ := os.ReadFile(target); string(b) != "OLD" {
		t.Errorf("not restored: %q", b)
	}
}

func TestDownloadThenInstall(t *testing.T) {
	skipJS(t)
	name := "cad-lite-1.5.0-windows-amd64.zip"
	f := newFakeGitHub(t, "v1.5.0", map[string][]byte{name: zipBytes(t, map[string]string{"cad-lite-windows-amd64/cad-lite.exe": "NEW"}, nil)})
	c := f.client("windows", "amd64")
	rel, ok, err := c.Check(context.Background(), "1.4.0")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	path, err := c.Download(context.Background(), rel, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "cad-lite.exe")
	os.WriteFile(exe, []byte("OLD"), 0o755)
	if _, err := Install(path, "windows", exe); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" {
		t.Errorf("exe = %q", b)
	}
}

func TestRelaunchStartsProcess(t *testing.T) {
	if runtime.GOOS == "windows" || !Supported() {
		t.Skip()
	}
	out := filepath.Join(t.TempDir(), "out")
	script := filepath.Join(t.TempDir(), "app.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\" > "+out+"\n"), 0o755)
	if err := Relaunch(Plan{GOOS: runtime.GOOS, Exe: script, Args: []string{"-open", "a b.dxf"}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if b, err := os.ReadFile(out); err == nil && strings.TrimSpace(string(b)) == "-open a b.dxf" {
			return
		}
		sleep()
	}
	t.Error("relaunched process did not run with its arguments")
}

func sleep() { time.Sleep(20 * time.Millisecond) }

func TestUpdateEndToEnd(t *testing.T) {
	skipJS(t)
	name := "cad-lite-1.5.0-linux-amd64.tar.gz"
	f := newFakeGitHub(t, "v1.5.0", map[string][]byte{name: tgzBytes(t, map[string]string{"d/bin/cad-lite": "NEW"})})
	c := f.client("linux", "amd64")
	c.Exe = filepath.Join(t.TempDir(), "cad-lite")
	os.WriteFile(c.Exe, []byte("OLD"), 0o755)
	rel, ok, err := c.Check(context.Background(), "1.0.0")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	plan, err := c.Update(context.Background(), rel, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(c.Exe); string(b) != "NEW" || plan.Exe != c.Exe || plan.Version != "1.5.0" {
		t.Errorf("exe %q plan %+v", b, plan)
	}
	// A tampered download leaves the installation alone.
	f.sums = strings.Repeat("0", 64) + "  " + name + "\n"
	os.WriteFile(c.Exe, []byte("OLD"), 0o755)
	if _, err := c.Update(context.Background(), rel, nil); !errors.Is(err, ErrChecksum) {
		t.Errorf("err = %v", err)
	}
	if b, _ := os.ReadFile(c.Exe); string(b) != "OLD" {
		t.Errorf("exe changed after a failed verification: %q", b)
	}
}

func TestAssetSelectionAcceptsLegacyPrefix(t *testing.T) {
	rel := Release{Version: "1.2.3", Assets: []Asset{
		{Name: "go-draw-1.2.3-linux-amd64.tar.gz"}, {Name: "go-draw-1.2.3-macos-universal.zip"},
	}}
	if a, ok := rel.AssetFor("linux", "amd64"); !ok || a.Name != "go-draw-1.2.3-linux-amd64.tar.gz" {
		t.Errorf("legacy linux asset = %v %v", a, ok)
	}
	if a, ok := rel.AssetFor("darwin", "arm64"); !ok || a.Name != "go-draw-1.2.3-macos-universal.zip" {
		t.Errorf("legacy macOS asset = %v %v", a, ok)
	}
	if _, ok := rel.AssetFor("windows", "amd64"); ok {
		t.Error("found a Windows asset that is not there")
	}
	// The new name wins when a release carries both.
	rel.Assets = append(rel.Assets, Asset{Name: "cad-lite-1.2.3-linux-amd64.tar.gz"})
	if a, _ := rel.AssetFor("linux", "amd64"); a.Name != "cad-lite-1.2.3-linux-amd64.tar.gz" {
		t.Errorf("preferred asset = %q", a.Name)
	}
}

func TestInstallOverLegacyNamedInstallation(t *testing.T) {
	// A bundle installed as go-draw.app, with the go-draw binary, updated from a cad-lite archive.
	root := t.TempDir()
	app := filepath.Join(root, "go-draw.app")
	exe := filepath.Join(app, "Contents", "MacOS", "go-draw")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("OLD"), 0o755)
	z := writeTemp(t, "m.zip", macZip(t, "NEW"))
	plan, err := Install(z, "darwin", exe)
	if err != nil {
		t.Fatal(err)
	}
	if plan.App != app {
		t.Errorf("plan.App = %q, want the installed bundle", plan.App)
	}
	if b, _ := os.ReadFile(filepath.Join(app, "Contents", "MacOS", "cad-lite")); string(b) != "NEW" {
		t.Errorf("new binary = %q", b)
	}

	// A plain binary named go-draw, updated from an archive that holds cad-lite.
	bin := filepath.Join(root, "bin", "go-draw")
	os.MkdirAll(filepath.Dir(bin), 0o755)
	os.WriteFile(bin, []byte("OLD"), 0o755)
	g := writeTemp(t, "x.tar.gz", tgzBytes(t, map[string]string{"d/bin/cad-lite": "NEW"}))
	if _, err := Install(g, "linux", bin); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(bin); string(b) != "NEW" {
		t.Errorf("binary = %q", b)
	}

	// An archive published under the old name still installs.
	os.WriteFile(bin, []byte("OLD"), 0o755)
	g = writeTemp(t, "y.tar.gz", tgzBytes(t, map[string]string{"d/bin/go-draw": "OLDNAME"}))
	if _, err := Install(g, "linux", bin); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(bin); string(b) != "OLDNAME" {
		t.Errorf("binary = %q", b)
	}
}

func TestDisabledReadsLegacyEnvName(t *testing.T) {
	t.Setenv("CADLITE_NO_UPDATE", "")
	os.Unsetenv("CADLITE_NO_UPDATE")
	t.Setenv("GODRAW_NO_UPDATE", "1")
	if !Disabled() {
		t.Error("GODRAW_NO_UPDATE=1 did not disable updates")
	}
	t.Setenv("CADLITE_NO_UPDATE", "0")
	if Disabled() {
		t.Error("CADLITE_NO_UPDATE=0 must win over the legacy name")
	}
}
