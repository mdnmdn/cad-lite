//go:build windows

package ui

import (
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const createNewConsole = 0x00000010

// newConsole marks a terminal for a new console, with an optional verbatim
// command line for cmd.exe's special quoting rules.
func newConsole(cmd *exec.Cmd, cmdLine string) {
	if cmdLine == "" {
		parts := make([]string, len(cmd.Args))
		for i, arg := range cmd.Args {
			parts[i] = syscall.EscapeArg(arg)
		}
		cmdLine = strings.Join(parts, " ")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole, CmdLine: cmdLine}
}

// startTerminal lets Windows initialize the new console's standard handles.
// os/exec always sets STARTF_USESTDHANDLES, which redirects nil streams to NUL
// even with CREATE_NEW_CONSOLE, leaving interactive programs in a blank window.
func startTerminal(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&createNewConsole == 0 {
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}
	if cmd.Err != nil {
		return cmd.Err
	}
	app, err := syscall.UTF16PtrFromString(cmd.Path)
	if err != nil {
		return err
	}
	line, err := syscall.UTF16PtrFromString(cmd.SysProcAttr.CmdLine)
	if err != nil {
		return err
	}
	var dir *uint16
	if cmd.Dir != "" {
		fullDir, err := filepath.Abs(cmd.Dir)
		if err != nil {
			return err
		}
		dir, err = syscall.UTF16PtrFromString(fullDir)
		if err != nil {
			return err
		}
	}
	env := cmd.Environ()
	for _, value := range cmd.Env {
		if strings.IndexByte(value, 0) >= 0 {
			return errors.New("terminal environment contains NUL")
		}
	}
	slices.SortFunc(env, func(a, b string) int { return strings.Compare(strings.ToUpper(a), strings.ToUpper(b)) })
	block := utf16.Encode([]rune(strings.Join(env, "\x00") + "\x00\x00"))
	startup := syscall.StartupInfo{Cb: uint32(unsafe.Sizeof(syscall.StartupInfo{}))}
	var process syscall.ProcessInformation
	err = syscall.CreateProcess(app, line, nil, nil, false,
		createNewConsole|syscall.CREATE_UNICODE_ENVIRONMENT, &block[0], dir, &startup, &process)
	if err != nil {
		return err
	}
	// Windows owns the independent console process; retaining handles is unnecessary.
	_ = syscall.CloseHandle(process.Thread)
	_ = syscall.CloseHandle(process.Process)
	return nil
}
