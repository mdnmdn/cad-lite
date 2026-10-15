package ui

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestWindowsTerminalEncodedCommand(t *testing.T) {
	dir := t.TempDir()
	argv := []string{`C:\Users\O'Brien\工具\opencode.exe`, "auth", "login", `a"b`, `ends\`, "$(exit)", ""}
	cmd, name, err := terminalCommandArgs("windows", dir, "", argv, func(string) (string, error) { return `C:\Windows\PowerShell Space\powershell.exe`, nil })
	if err != nil {
		t.Fatal(err)
	}
	if name != "PowerShell" || cmd.Dir != dir {
		t.Fatalf("terminal %q dir %q", name, cmd.Dir)
	}
	if len(cmd.Args) != 5 || strings.Join(cmd.Args[1:4], " ") != "-NoProfile -NoExit -EncodedCommand" {
		t.Fatalf("PowerShell startup: %q", cmd.Args)
	}
	encoded, err := base64.StdEncoding.DecodeString(cmd.Args[4])
	if err != nil {
		t.Fatal(err)
	}
	units := make([]uint16, len(encoded)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(encoded[i*2:])
	}
	want := `& 'C:\Users\O''Brien\工具\opencode.exe' 'auth' 'login' 'a"b' 'ends\' '$(exit)' ''`
	if got := string(utf16.Decode(units)); got != want {
		t.Fatalf("decoded command = %q, want %q", got, want)
	}
}

func TestWindowsTerminalFallback(t *testing.T) {
	cmd, name, err := terminalCommandArgs("windows", "", "", []string{`C:\Tools Space\opencode.exe`, "auth", "login"}, func(string) (string, error) { return "", os.ErrNotExist })
	if err != nil || name != "cmd" || cmd.Args[1] != "/k" || cmd.Args[2] != `"C:\Tools Space\opencode.exe" "auth" "login"` {
		t.Fatalf("fallback: %v %q %v", cmd, name, err)
	}
}
