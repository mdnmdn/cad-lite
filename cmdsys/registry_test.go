package cmdsys

import (
	"reflect"
	"testing"
)

func TestAliasesOfShortestFirst(t *testing.T) {
	r := NewRegistry()
	r.Alias("linea", "LINE")
	r.Alias("L", "line")
	r.Alias("LI", "LIST")
	r.Alias("-L", "-LINE")
	if got, want := r.AliasesOf("line"), []string{"L", "LINEA"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AliasesOf(line) = %v, want %v", got, want)
	}
	if got := r.AliasesOf("REDO"); len(got) != 0 {
		t.Errorf("AliasesOf(REDO) = %v, want none", got)
	}
}
