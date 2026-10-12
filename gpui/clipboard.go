package gpui

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

type clipTool struct {
	read, write []string
}

func clipboardTools() []clipTool {
	switch runtime.GOOS {
	case "darwin":
		return []clipTool{{[]string{"pbpaste"}, []string{"pbcopy"}}}
	case "windows":
		return []clipTool{{
			[]string{"powershell", "-NoProfile", "-Command", "Get-Clipboard"},
			[]string{"powershell", "-NoProfile", "-Command", "$input | Set-Clipboard"},
		}}
	default:
		return []clipTool{
			{[]string{"wl-paste", "--no-newline"}, []string{"wl-copy"}},
			{[]string{"xclip", "-selection", "clipboard", "-o"}, []string{"xclip", "-selection", "clipboard"}},
			{[]string{"xsel", "--clipboard", "--output"}, []string{"xsel", "--clipboard", "--input"}},
		}
	}
}

func firstClipTool() (clipTool, error) {
	for _, t := range clipboardTools() {
		if _, err := exec.LookPath(t.read[0]); err == nil {
			return t, nil
		}
	}
	return clipTool{}, errors.New("gpui: no clipboard tool available")
}

// ClipboardAvailable reports whether an OS clipboard tool was found.
func ClipboardAvailable() bool {
	_, err := firstClipTool()
	return err == nil
}

// ClipboardRead returns the system clipboard text, via the OS clipboard tool.
func ClipboardRead() (string, error) {
	t, err := firstClipTool()
	if err != nil {
		return "", err
	}
	out, err := exec.Command(t.read[0], t.read[1:]...).Output()
	if err != nil {
		return "", err
	}
	s := string(out)
	if runtime.GOOS == "windows" {
		s = strings.TrimSuffix(strings.TrimSuffix(s, "\n"), "\r")
	}
	return s, nil
}

// ClipboardWrite replaces the system clipboard text with s, passing it to the
// OS clipboard tool on stdin.
func ClipboardWrite(s string) error {
	t, err := firstClipTool()
	if err != nil {
		return err
	}
	cmd := exec.Command(t.write[0], t.write[1:]...)
	cmd.Stdin = bytes.NewReader([]byte(s))
	return cmd.Run()
}
