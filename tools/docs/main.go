// Command docs maintains the _docs/ library: it lints documents against the
// rules in _docs/_meta/librarian.md, lists the documents a change touches,
// reports drift between documents and the code they anchor, and regenerates
// the INDEX.md catalogue and the code map.
//
//	go run ./tools/docs lint            checks L1, L2, L4, L5, L7, L8, L9, L10
//	go run ./tools/docs touched [path…] documents anchored to the paths (default: git diff)
//	go run ./tools/docs drift           documents whose anchors changed after `verified`
//	go run ./tools/docs index           regenerates the INDEX.md catalogue and tech/code-map.md
//
// The justfile wraps these as `just docs-lint`, `docs-touched`, `docs-drift`
// and `docs-index`. Run it from the repository root.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const root = "_docs"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: docs lint|touched|drift|index")
		os.Exit(2)
	}
	docs, err := load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "lint":
		problems := lint(docs)
		for _, p := range problems {
			fmt.Println(p)
		}
		if len(problems) > 0 {
			fmt.Printf("%d problem(s)\n", len(problems))
			os.Exit(1)
		}
		fmt.Printf("%d documents, no problems\n", len(docs))
	case "touched":
		touched(docs, os.Args[2:])
	case "drift":
		drift(docs)
	case "index":
		if err := index(docs); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command", os.Args[1])
		os.Exit(2)
	}
}

// document is one parsed markdown file under _docs/.
type document struct {
	path  string // repo-relative
	rel   string // relative to _docs/
	front map[string]string
	lists map[string][]string
	body  []string
	lines int
}

func (d *document) folder() string {
	if i := strings.Index(d.rel, "/"); i >= 0 {
		return d.rel[:i]
	}
	return ""
}

// managed reports whether the document carries frontmatter and obeys the
// rules: everything except inbox/ and design/ material.
func (d *document) managed() bool {
	f := d.folder()
	return f != "inbox" && f != "design"
}

func load() ([]*document, error) {
	var docs []*document
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		d := &document{path: filepath.ToSlash(path), rel: filepath.ToSlash(rel), front: map[string]string{}, lists: map[string][]string{}}
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		d.lines = len(lines)
		body := lines
		if len(lines) > 0 && lines[0] == "---" {
			for i := 1; i < len(lines); i++ {
				if lines[i] == "---" {
					body = lines[i+1:]
					break
				}
				k, v, ok := strings.Cut(lines[i], ":")
				if !ok {
					continue
				}
				k, v = strings.TrimSpace(k), strings.TrimSpace(v)
				if c := strings.Index(v, " #"); c >= 0 {
					v = strings.TrimSpace(v[:c])
				}
				d.front[k] = v
				if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
					for _, item := range strings.Split(strings.Trim(v, "[]"), ",") {
						if item = strings.TrimSpace(item); item != "" {
							d.lists[k] = append(d.lists[k], item)
						}
					}
				}
			}
		}
		d.body = body
		docs = append(docs, d)
		return nil
	})
	sort.Slice(docs, func(i, j int) bool { return docs[i].rel < docs[j].rel })
	return docs, err
}

var (
	linkRe    = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	headingRe = regexp.MustCompile(`(?i)^#+\s*(open questions|known gaps?)\b`)
	banned    = []string{"now", "already", "no longer", "used to", "previously", "currently", "not yet", "will be", "has been added", "was changed"}
	bannedRe  = regexp.MustCompile(`(?i)\b(` + strings.Join(banned, "|") + `)\b`)
	inlineRe  = regexp.MustCompile("`[^`]*`")
	prefixes  = map[string]string{"product": "prod-", "features": "feat-", "tech": "tech-", "_meta": "meta-", "wip": "wip-"}
	// exempt from the length bands (read by lookup, not read through).
	exempt  = []string{"INDEX.md", "backlog.md", "tech/decisions.md", "tech/code-map.md", "_meta/feedback.md", "_meta/review-log.md"}
	timeful = []string{"product", "features", "tech"}
)

func lint(docs []*document) []string {
	var out []string
	add := func(d *document, check, msg string, args ...any) {
		out = append(out, fmt.Sprintf("%s: %s %s", d.path, check, fmt.Sprintf(msg, args...)))
	}
	ids := map[string]*document{}
	for _, d := range docs {
		if !d.managed() || d.rel == "INDEX.md" || d.rel == "backlog.md" {
			continue
		}
		id := d.front["id"]
		if id == "" {
			add(d, "L1", "missing frontmatter id")
			continue
		}
		if o, dup := ids[id]; dup {
			add(d, "L1", "id %q also used by %s", id, o.path)
		}
		ids[id] = d
		if p, ok := prefixes[d.folder()]; ok && !strings.HasPrefix(id, p) {
			add(d, "L1", "id %q should start with %q", id, p)
		}
		for _, k := range []string{"title", "kind", "status", "summary", "read_when", "updated"} {
			if d.front[k] == "" {
				add(d, "L1", "missing frontmatter %s", k)
			}
		}
	}
	index := docByRel(docs, "INDEX.md")
	for _, d := range docs {
		if !d.managed() {
			continue
		}
		for _, dep := range d.lists["depends_on"] {
			if ids[dep] == nil {
				add(d, "L1", "depends_on %q does not resolve", dep)
			}
		}
		for _, a := range d.lists["code_anchors"] {
			if _, err := os.Stat(a); err != nil {
				add(d, "L2", "code anchor %s does not exist", a)
			}
		}
		counts := map[string]int{}
		inFence, fenceLen, fenced := false, 0, 0
		for i, line := range d.body {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				if inFence && fenceLen > 20 {
					add(d, "L4", "fence ending at body line %d is %d lines (max 20)", i+1, fenceLen)
				}
				inFence, fenceLen = !inFence, 0
				continue
			}
			if inFence {
				fenceLen++
				fenced++
				continue
			}
			for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
				target := m[1]
				if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
					continue
				}
				file, _, _ := strings.Cut(target, "#")
				resolved := filepath.Clean(filepath.Join(filepath.Dir(d.path), file))
				counts[resolved]++
				if _, err := os.Stat(resolved); err != nil {
					add(d, "L2", "link %s does not resolve", target)
				}
			}
			if headingRe.MatchString(line) && d.front["status"] == "current" {
				add(d, "L8", "heading %q belongs in backlog.md", strings.TrimSpace(line))
			}
			if slices.Contains(timeful, d.folder()) && d.front["status"] == "current" {
				if m := bannedRe.FindString(inlineRe.ReplaceAllString(line, "")); m != "" {
					add(d, "L5", "body line %d uses %q", i+1, m)
				}
			}
		}
		for target, n := range counts {
			if n > 3 && d.rel != "INDEX.md" && d.rel != "backlog.md" {
				add(d, "L9", "links %s %d times (max 3)", target, n)
			}
		}
		if !slices.Contains(exempt, d.rel) {
			if d.lines > 500 {
				add(d, "L4", "%d lines (ceiling 500)", d.lines)
			}
			if d.lines > 0 && fenced*100 > 15*d.lines {
				add(d, "L4", "fenced code is %d%% of the document (max 15%%)", fenced*100/d.lines)
			}
		}
		if index != nil && d != index && !linksTo(index, d) {
			add(d, "L7", "not listed in INDEX.md")
		}
	}
	if index == nil || !strings.Contains(strings.Join(index.body, "\n"), "_meta/authoring.md") {
		out = append(out, root+"/INDEX.md: L10 the pointer to _meta/authoring.md is missing")
	}
	if b, err := os.ReadFile("AGENTS.md"); err != nil || !strings.Contains(string(b), "_docs/_meta/authoring.md") {
		out = append(out, "AGENTS.md: L10 the update duty and the pointer to _docs/_meta/authoring.md are missing")
	}
	return out
}

func docByRel(docs []*document, rel string) *document {
	for _, d := range docs {
		if d.rel == rel {
			return d
		}
	}
	return nil
}

func linksTo(from, to *document) bool {
	for _, line := range from.body {
		for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
			file, _, _ := strings.Cut(m[1], "#")
			if filepath.Clean(filepath.Join(filepath.Dir(from.path), file)) == filepath.Clean(to.path) {
				return true
			}
		}
	}
	return false
}

func git(args ...string) []string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil
	}
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// anchored reports whether path is one of d's anchors or lies under one.
func anchored(d *document, path string) bool {
	for _, a := range d.lists["code_anchors"] {
		if path == a || strings.HasPrefix(path, strings.TrimSuffix(a, "/")+"/") {
			return true
		}
	}
	return false
}

func touched(docs []*document, paths []string) {
	if len(paths) == 0 {
		paths = append(git("diff", "--name-only", "HEAD"), git("ls-files", "--others", "--exclude-standard")...)
	}
	seen := map[string]bool{}
	for _, p := range paths {
		for _, d := range docs {
			if anchored(d, filepath.ToSlash(p)) && !seen[d.path] {
				seen[d.path] = true
				fmt.Printf("%s\t(%s)\n", d.path, d.front["read_when"])
			}
		}
	}
	if len(seen) == 0 {
		fmt.Println("no anchored documents; if the change is user-visible, see _docs/_meta/authoring.md §2")
	}
}

func drift(docs []*document) {
	n := 0
	for _, d := range docs {
		verified := d.front["verified"]
		for _, a := range d.lists["code_anchors"] {
			last := git("log", "-1", "--format=%cs", "--", a)
			if len(last) == 1 && verified != "" && last[0] > verified {
				fmt.Printf("%s: %s changed %s, verified %s\n", d.path, a, last[0], verified)
				n++
			}
		}
	}
	fmt.Printf("%d drifted anchor(s)\n", n)
}

const (
	startMark = "<!-- docs-index:start -->"
	endMark   = "<!-- docs-index:end -->"
)

// replaceBetween swaps the text between the docs-index markers of path.
func replaceBetween(path, content string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	i, j := strings.Index(s, startMark), strings.Index(s, endMark)
	if i < 0 || j < i {
		return fmt.Errorf("%s: docs-index markers not found", path)
	}
	s = s[:i+len(startMark)] + "\n" + content + s[j:]
	return os.WriteFile(path, []byte(s), 0o644)
}

func index(docs []*document) error {
	sections := []struct{ folder, title string }{
		{"product", "Product"}, {"features", "Features"}, {"tech", "Technical"}, {"wip", "Work in progress"}, {"_meta", "About this library"},
	}
	var b strings.Builder
	for _, s := range sections {
		var rows []string
		for _, d := range docs {
			if d.folder() == s.folder && d.front["id"] != "" {
				rows = append(rows, fmt.Sprintf("| [%s](%s) | %s | %s |", d.front["title"], d.rel, d.front["summary"], d.front["read_when"]))
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n### %s\n\n| Document | Summary | Read when |\n|---|---|---|\n%s\n", s.title, strings.Join(rows, "\n"))
	}
	b.WriteString("\n")
	if err := replaceBetween(filepath.Join(root, "INDEX.md"), b.String()); err != nil {
		return err
	}
	return codeMap()
}

// codeMap regenerates tech/code-map.md from the Go package doc comments.
func codeMap() error {
	pkgs := git("-c", "core.quotepath=off", "ls-files", "*.go")
	dirs := map[string]bool{}
	for _, p := range pkgs {
		dir := filepath.Dir(p)
		if strings.HasPrefix(dir, "internal/gpui/examples") {
			continue
		}
		dirs[dir] = true
	}
	out, err := exec.Command("go", "list", "-f", "{{.Dir}}\t{{.ImportPath}}\t{{.Doc}}", "./...").Output()
	if err != nil {
		return fmt.Errorf("go list: %w", err)
	}
	wd, _ := os.Getwd()
	var b strings.Builder
	b.WriteString("\n| Package | Purpose |\n|---|---|\n")
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 3)
		if len(parts) < 3 {
			continue
		}
		rel, _ := filepath.Rel(wd, parts[0])
		if !dirs[filepath.ToSlash(rel)] {
			continue
		}
		fmt.Fprintf(&b, "| `%s` | %s |\n", parts[1], parts[2])
	}
	b.WriteString("\n")
	return replaceBetween(filepath.Join(root, "tech", "code-map.md"), b.String())
}
