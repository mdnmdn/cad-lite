package main

import (
	"reflect"
	"testing"
)

func TestLaunchArgsDropsTheProcessSerialNumber(t *testing.T) {
	got := launchArgs([]string{"-psn_0_123456", "-width", "900", "plan.dxf"})
	want := []string{"-width", "900", "plan.dxf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("launchArgs = %q, want %q", got, want)
	}
}

func TestFileArgs(t *testing.T) {
	got, err := fileArgs("", []string{"a.dxf", "b.dwg"})
	if err != nil || !reflect.DeepEqual(got, []string{"a.dxf", "b.dwg"}) {
		t.Fatalf("fileArgs = %q, %v", got, err)
	}
	if got, err := fileArgs("x.dxf", nil); err != nil || len(got) != 0 {
		t.Fatalf("-open alone: %q, %v", got, err)
	}
	if _, err := fileArgs("x.dxf", []string{"a.dxf"}); err == nil {
		t.Fatal("-open with a file argument should be refused")
	}
}
