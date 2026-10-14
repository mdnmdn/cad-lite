package updater

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed semantic version (https://semver.org), without a
// leading "v". Build metadata after "+" is kept but never compared.
type Version struct {
	// Major, Minor and Patch are the numeric core.
	Major, Minor, Patch uint64
	// Pre holds the dot-separated pre-release identifiers ("rc", "1").
	Pre []string
	// Build is the build metadata after "+", empty when absent.
	Build string
}

// ParseVersion parses "1.2.3", "v1.2.3", "1.2.3-rc.1" and "0.0.0-ci+abc1234".
// Anything else, including "dev", is an error.
func ParseVersion(s string) (Version, error) {
	orig := s
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	var v Version
	if i := strings.IndexByte(s, '+'); i >= 0 {
		v.Build, s = s[i+1:], s[:i]
		if v.Build == "" {
			return Version{}, fmt.Errorf("updater: version %q: empty build metadata", orig)
		}
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre := s[i+1:]
		s = s[:i]
		if pre == "" {
			return Version{}, fmt.Errorf("updater: version %q: empty pre-release", orig)
		}
		v.Pre = strings.Split(pre, ".")
		for _, id := range v.Pre {
			if id == "" {
				return Version{}, fmt.Errorf("updater: version %q: empty pre-release identifier", orig)
			}
		}
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("updater: version %q is not major.minor.patch", orig)
	}
	var n [3]uint64
	for i, p := range parts {
		x, err := strconv.ParseUint(p, 10, 32)
		if err != nil || (len(p) > 1 && p[0] == '0') {
			return Version{}, fmt.Errorf("updater: version %q: bad number %q", orig, p)
		}
		n[i] = x
	}
	v.Major, v.Minor, v.Patch = n[0], n[1], n[2]
	return v, nil
}

// String renders v as "major.minor.patch[-pre][+build]".
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// Compare orders versions by semver precedence: -1 when v is older than o,
// 0 when equal, 1 when newer. Build metadata is ignored.
func (v Version) Compare(o Version) int {
	for _, p := range [][2]uint64{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(v.Pre) == 0 && len(o.Pre) == 0:
		return 0
	case len(v.Pre) == 0:
		return 1
	case len(o.Pre) == 0:
		return -1
	}
	for i := 0; i < len(v.Pre) && i < len(o.Pre); i++ {
		if c := comparePre(v.Pre[i], o.Pre[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(v.Pre) < len(o.Pre):
		return -1
	case len(v.Pre) > len(o.Pre):
		return 1
	}
	return 0
}

func comparePre(a, b string) int {
	an, aerr := strconv.ParseUint(a, 10, 64)
	bn, berr := strconv.ParseUint(b, 10, 64)
	switch {
	case aerr == nil && berr == nil:
		if an < bn {
			return -1
		} else if an > bn {
			return 1
		}
		return 0
	case aerr == nil:
		return -1 // numeric identifiers sort before alphanumeric ones
	case berr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

// Updatable reports whether a build with this version string may be replaced
// by a release: it must be valid semver without build metadata. "dev" builds
// and CI builds ("0.0.0-ci+abc1234") are never updated automatically.
func Updatable(current string) bool {
	v, err := ParseVersion(current)
	return err == nil && v.Build == ""
}
