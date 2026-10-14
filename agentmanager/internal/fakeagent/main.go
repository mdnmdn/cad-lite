// Command fakeagent stands in for the opencode, claude and codex CLIs in tests. The first
// recognised argument selects the protocol: acp, -p (claude) or app-server (codex).
package main

import (
	"fmt"
	"os"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	sc, err := LoadScenario(os.Getenv("FAKEAGENT_SCENARIO"))
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
