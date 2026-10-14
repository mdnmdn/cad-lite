// Command i18ncheck verifies Cad-lite's message catalogs and finds user-facing string literals
// that are not catalog keys yet.
//
//	go run ./i18n/cmd/i18ncheck                       # catalog parity and placeholders only
//	go run ./i18n/cmd/i18ncheck -scan ui,cmdsys       # also scan packages for literals
//	go run ./i18n/cmd/i18ncheck -scan ui -strict      # exit 1 when a literal is found
//
// The scan reports string literals at known UI call sites: gpui.NewText and NewTextInput,
// Prompt{Message: ...}, Prompt keyword lists, Printf/print/errorf style output calls and the
// button/section helpers of the ui package. A literal whose text equals an English catalog value
// is reported with the key to use; any other literal is reported as unkeyed. Test files are
// skipped. Directories are scanned recursively.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"cad-lite/i18n"
)

// callArgs maps a function or method name to the argument positions that are user-facing text.
var callArgs = map[string][]int{
	"NewText":      {0},
	"NewTextInput": {0},
	"Printf":       {0},
	"print":        {0},
	"button":       {1},
	"smallButton":  {1},
	"sectionTitle": {0},
	"settingRow":   {0},
	"heading":      {0},
	"flag":         {1},
	"warnOnce":     {1},
}

// ignored are literals that are not prose.
var ignored = map[string]bool{"AI": true, "x": true, "+": true, "Cad-lite": true, "": true}

type finding struct {
	pos  token.Position
	text string
	site string
	key  string
}

func main() {
	scan := flag.String("scan", "", "comma-separated directories to scan for unkeyed literals")
	strict := flag.Bool("strict", false, "exit 1 when the scan finds an unkeyed literal")
	known := flag.Bool("known", false, "also list literals that already have a catalog key")
	flag.Parse()

	failed := false
	if problems := i18n.Verify(); len(problems) > 0 {
		failed = true
		fmt.Fprintf(os.Stderr, "catalog problems (%d):\n", len(problems))
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "  "+p)
		}
	} else {
		fmt.Printf("catalogs ok: %d keys, %d arrays per language\n", len(i18n.Keys(i18n.English)), len(i18n.ListKeys(i18n.English)))
	}

	if *scan != "" {
		byText := map[string]string{}
		for _, k := range i18n.Keys(i18n.English) {
			s, _ := i18n.Raw(i18n.English, k)
			if prev, dup := byText[s]; !dup || k < prev {
				byText[s] = k
			}
		}
		var all []finding
		for _, dir := range strings.Split(*scan, ",") {
			fs, err := scanDir(strings.TrimSpace(dir), byText)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			all = append(all, fs...)
		}
		unkeyed := 0
		for _, f := range all {
			if f.key != "" {
				if *known {
					fmt.Printf("%s: %q  [%s]  -> %s\n", f.pos, f.text, f.site, f.key)
				}
				continue
			}
			unkeyed++
			fmt.Printf("%s: %q  [%s]  (no key)\n", f.pos, f.text, f.site)
		}
		fmt.Printf("scan: %d literals with a key, %d without\n", len(all)-unkeyed, unkeyed)
		if unkeyed > 0 && *strict {
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

func scanDir(dir string, byText map[string]string) ([]finding, error) {
	var out []finding
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		out = append(out, scanFile(fset, f, byText)...)
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].pos, out[j].pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		return a.Line < b.Line
	})
	return out, err
}

func scanFile(fset *token.FileSet, f *ast.File, byText map[string]string) []finding {
	var out []finding
	add := func(e ast.Expr, site string) {
		for _, lit := range leadingLiterals(e) {
			s, err := strconv.Unquote(lit.Value)
			if err != nil || !isProse(s) {
				continue
			}
			out = append(out, finding{pos: fset.Position(lit.Pos()), text: s, site: site, key: byText[s]})
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			name := calleeName(n.Fun)
			for _, i := range callArgs[name] {
				if i < len(n.Args) {
					add(n.Args[i], name+"(...)")
				}
			}
		case *ast.KeyValueExpr:
			if k, ok := n.Key.(*ast.Ident); ok {
				switch k.Name {
				case "Message":
					add(n.Value, "Prompt.Message")
				case "Keywords":
					if cl, ok := n.Value.(*ast.CompositeLit); ok {
						for _, el := range cl.Elts {
							add(el, "Prompt.Keywords (id; display comes from kw.*)")
						}
					}
				}
			}
		}
		return true
	})
	return out
}

func calleeName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}

// leadingLiterals returns the string literals of e that carry prose: e itself, the operands of a
// "+" chain, and the format argument of a nested fmt.Sprintf.
func leadingLiterals(e ast.Expr) []*ast.BasicLit {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			return []*ast.BasicLit{e}
		}
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			return append(leadingLiterals(e.X), leadingLiterals(e.Y)...)
		}
	case *ast.ParenExpr:
		return leadingLiterals(e.X)
	case *ast.CallExpr:
		if calleeName(e.Fun) == "Sprintf" && len(e.Args) > 0 {
			return leadingLiterals(e.Args[0])
		}
	}
	return nil
}

// isProse reports whether s contains a letter and is not a lone symbol or identifier-like token.
func isProse(s string) bool {
	if ignored[s] {
		return false
	}
	letters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters >= 2
}
