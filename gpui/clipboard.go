package gpui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// clipTimeout bounds every clipboard tool invocation so a hung tool (a dead
// X server, a stuck PowerShell) cannot freeze the caller.
const clipTimeout = time.Second

type clipTool struct {
	read, write []string
}

// psUTF8 is a BOM-less UTF-8 encoding expression for PowerShell.
const psUTF8 = "(New-Object System.Text.UTF8Encoding $false)"

func clipboardTools(goos string, getenv func(string) string) []clipTool {
	switch goos {
	case "darwin":
		return []clipTool{{[]string{"pbpaste"}, []string{"pbcopy"}}}
	case "windows":
		return []clipTool{{
			[]string{"powershell", "-NoProfile", "-NonInteractive", "-Command",
				"[Console]::OutputEncoding=" + psUTF8 + "; $t=Get-Clipboard -Raw; if ($t) { [Console]::Out.Write($t) }"},
			[]string{"powershell", "-NoProfile", "-NonInteractive", "-Command",
				"[Console]::InputEncoding=" + psUTF8 + "; $s=[Console]::In.ReadToEnd(); if ($s.Length -gt 0) { Set-Clipboard -Value $s }"},
		}}
	default:
		var ts []clipTool
		if getenv("WAYLAND_DISPLAY") != "" {
			ts = append(ts, clipTool{[]string{"wl-paste", "--no-newline"}, []string{"wl-copy"}})
		}
		return append(ts,
			clipTool{[]string{"xclip", "-selection", "clipboard", "-o"}, []string{"xclip", "-selection", "clipboard"}},
			clipTool{[]string{"xsel", "--clipboard", "--output"}, []string{"xsel", "--clipboard", "--input"}},
		)
	}
}

func systemClipTools() []clipTool { return clipboardTools(runtime.GOOS, os.Getenv) }

var errNoClipTool = errors.New("gpui: no clipboard tool available")

// runClip runs argv with stdin under a timeout, returning stdout when
// capture is set. Write tools must not capture: xclip and wl-copy fork a
// daemon that would inherit (and hold open) a stdout pipe.
func runClip(argv []string, stdin []byte, capture bool, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.WaitDelay = 100 * time.Millisecond
	cmd.Stdin = bytes.NewReader(stdin)
	var out bytes.Buffer
	if capture {
		cmd.Stdout = &out
	}
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return out.Bytes(), err
}

// readWith tries each installed tool in order, falling through on error.
func readWith(tools []clipTool, timeout time.Duration) (string, error) {
	err := errNoClipTool
	for _, t := range tools {
		if _, lerr := exec.LookPath(t.read[0]); lerr != nil {
			continue
		}
		out, rerr := runClip(t.read, nil, true, timeout)
		if rerr == nil {
			return string(out), nil
		}
		err = rerr
	}
	return "", err
}

// writeWith tries each installed tool in order, falling through on error.
func writeWith(tools []clipTool, s string, timeout time.Duration) error {
	err := errNoClipTool
	for _, t := range tools {
		if _, lerr := exec.LookPath(t.write[0]); lerr != nil {
			continue
		}
		_, werr := runClip(t.write, []byte(s), false, timeout)
		if werr == nil {
			return nil
		}
		err = werr
	}
	return err
}

// ClipboardAvailable reports whether an OS clipboard tool was found.
func ClipboardAvailable() bool {
	for _, t := range systemClipTools() {
		if _, err := exec.LookPath(t.read[0]); err == nil {
			return true
		}
	}
	return false
}

// ClipboardRead returns the system clipboard text via the first OS clipboard
// tool that succeeds within the timeout. It blocks; UI code should call it
// off the main loop.
func ClipboardRead() (string, error) { return readWith(systemClipTools(), clipTimeout) }

// ClipboardWrite replaces the system clipboard text with s, passing it on
// stdin to the first OS clipboard tool that succeeds within the timeout. It
// blocks; UI code should call it off the main loop.
func ClipboardWrite(s string) error { return writeWith(systemClipTools(), s, clipTimeout) }
