package proc

import (
	"os/exec"
	"reflect"
	"runtime"
	"syscall"
	"testing"
)

func TestHideWindowPreservesProcessAttributes(t *testing.T) {
	cmd := exec.Command("unused")
	attrs := &syscall.SysProcAttr{}
	cmd.SysProcAttr = attrs
	before := *attrs
	HideWindow(cmd)
	if cmd.SysProcAttr != attrs {
		t.Fatal("existing process attributes replaced")
	}
	if runtime.GOOS != "windows" && !reflect.DeepEqual(*attrs, before) {
		t.Fatal("non-Windows process attributes changed")
	}
}
