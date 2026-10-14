package updater

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ChecksumFiles are the names a release's checksum list may have, in order of
// preference. The list is in sha256sum format.
var ChecksumFiles = []string{"SHA256SUMS.txt", "checksums.txt"}

// AssetName returns the archive name release.yml publishes for a platform:
// go-draw-<version>-macos-universal.zip, go-draw-<version>-windows-<arch>.zip
// and go-draw-<version>-linux-<arch>.tar.gz. ok is false for platforms that
// have no archive.
func AssetName(version, goos, goarch string) (name string, ok bool) {
	switch goos {
	case "darwin":
		return "go-draw-" + version + "-macos-universal.zip", true
	case "windows":
		if goarch == "amd64" || goarch == "arm64" {
			return "go-draw-" + version + "-windows-" + goarch + ".zip", true
		}
	case "linux":
		if goarch == "amd64" || goarch == "arm64" {
			return "go-draw-" + version + "-linux-" + goarch + ".tar.gz", true
		}
	}
	return "", false
}

// AssetFor selects the archive to install on goos/goarch. The installers
// (-setup.exe, .dmg) are never chosen.
func (r Release) AssetFor(goos, goarch string) (Asset, bool) {
	want, ok := AssetName(r.Version, goos, goarch)
	if !ok {
		return Asset{}, false
	}
	for _, a := range r.Assets {
		if a.Name == want {
			return a, true
		}
	}
	return Asset{}, false
}

// ChecksumAsset finds the release's checksum list.
func (r Release) ChecksumAsset() (Asset, bool) {
	for _, n := range ChecksumFiles {
		for _, a := range r.Assets {
			if a.Name == n {
				return a, true
			}
		}
	}
	return Asset{}, false
}

// ParseChecksums reads sha256sum output ("<hex>  <name>" or "<hex> *<name>")
// into a name to lowercase hex map.
func ParseChecksums(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sum, name, ok := strings.Cut(line, " ")
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if !ok || len(sum) != 64 || name == "" {
			return nil, fmt.Errorf("updater: malformed checksum line %q", line)
		}
		out[name] = strings.ToLower(sum)
	}
	return out, sc.Err()
}
