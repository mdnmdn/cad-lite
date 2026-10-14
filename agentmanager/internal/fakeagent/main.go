// Command fakeagent stands in for the opencode, claude and codex CLIs in tests. The first
// recognised argument selects the protocol: acp, -p (claude) or app-server (codex).
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	dumpArgv(args)
	path := os.Getenv("FAKEAGENT_SCENARIO")
	for i, a := range args {
		if a == "--scenario" && i+1 < len(args) {
			path = args[i+1]
		}
	}
	sc, err := LoadScenario(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent:", err)
		return 2
	}
	for _, a := range args {
		switch a {
		case "acp":
			return RunACP(sc, os.Stdin, os.Stdout)
		case "-p", "claude":
			return RunClaude(sc, os.Stdin, os.Stdout)
		case "app-server", "codex":
			return RunCodex(sc, os.Stdin, os.Stdout)
		}
	}
	fmt.Fprintln(os.Stderr, "fakeagent: no protocol argument (acp | -p | app-server)")
	return 2
}

// dumpArgv writes argv (one argument per line), then the sorted environment, to FAKEAGENT_ARGV so
// tests can assert exactly what a backend launched.
func dumpArgv(args []string) {
	p := os.Getenv("FAKEAGENT_ARGV")
	if p == "" {
		return
	}
	var b strings.Builder
	b.WriteString("ARGV\n")
	for _, a := range args {
		b.WriteString(a + "\n")
	}
	env := os.Environ()
	sort.Strings(env)
	b.WriteString("ENV\n")
	for _, e := range env {
		b.WriteString(e + "\n")
	}
	if wd, err := os.Getwd(); err == nil {
		b.WriteString("CWD\n" + wd + "\n")
	}
	_ = os.WriteFile(p, []byte(b.String()), 0o600)
}
