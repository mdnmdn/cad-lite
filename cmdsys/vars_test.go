package cmdsys_test

import (
	"errors"
	"testing"

	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

func newSession() *cmdsys.Session {
	v := render.DefaultView(800, 600)
	return cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
}

func TestVarsRegisterSetGet(t *testing.T) {
	v := cmdsys.NewVars()
	changed := 0
	v.Register(cmdsys.Var{Name: "pickbox", Kind: cmdsys.VarInt, Default: 3, Min: 0, Max: 50, OnChange: func(old, new any) {
		if old.(int) != 3 || new.(int) != 7 {
			t.Errorf("OnChange(%v, %v)", old, new)
		}
		changed++
	}})
	v.Register(cmdsys.Var{Name: "DISTANCE", Kind: cmdsys.VarReal, Default: 0.0, ReadOnly: true})
	v.Register(cmdsys.Var{Name: "LASTPOINT", Kind: cmdsys.VarPoint, Default: geom.Vec2{}})
	v.Register(cmdsys.Var{Name: "CLAYER", Kind: cmdsys.VarText, Default: "0"})
	var heard []string
	v.Subscribe(func(name string, _, _ any) { heard = append(heard, name) })

	if v.Int("PICKBOX") != 3 {
		t.Fatalf("PICKBOX default = %d (names are case-insensitive)", v.Int("PICKBOX"))
	}
	if err := v.Set("PICKBOX", 7); err != nil || v.Int("pickbox") != 7 || changed != 1 {
		t.Fatalf("Set: err %v, value %d, changed %d", err, v.Int("PICKBOX"), changed)
	}
	for _, c := range []struct {
		name string
		val  any
		want error
	}{
		{"PICKBOX", 99, cmdsys.ErrVarRange},
		{"PICKBOX", "x", cmdsys.ErrVarType},
		{"PICKBOX", 1.5, cmdsys.ErrVarType},
		{"DISTANCE", 1.0, cmdsys.ErrVarReadOnly},
		{"NOPE", 1, cmdsys.ErrVarUnknown},
	} {
		if err := v.Set(c.name, c.val); !errors.Is(err, c.want) {
			t.Errorf("Set(%s, %v) = %v, want %v", c.name, c.val, err, c.want)
		}
	}
	if v.Int("PICKBOX") != 7 {
		t.Fatal("a refused Set must not change the value")
	}
	if err := v.Store("DISTANCE", 12.5); err != nil || v.Real("DISTANCE") != 12.5 {
		t.Fatalf("Store on a read-only variable: %v, %v", err, v.Real("DISTANCE"))
	}
	if err := v.SetString("LASTPOINT", "3, 4"); err != nil || v.Point("LASTPOINT") != geom.V(3, 4) || v.String("LASTPOINT") != "3,4" {
		t.Fatalf("SetString point: %v %v", err, v.Point("LASTPOINT"))
	}
	if err := v.SetString("PICKBOX", "abc"); !errors.Is(err, cmdsys.ErrVarType) {
		t.Fatalf("SetString(abc) = %v", err)
	}
	if err := v.SetString("CLAYER", " WALLS "); err != nil || v.Text("CLAYER") != "WALLS" {
		t.Fatalf("SetString text: %v %q", err, v.Text("CLAYER"))
	}
	if len(heard) != 4 { // PICKBOX, DISTANCE, LASTPOINT, CLAYER
		t.Fatalf("listeners heard %v", heard)
	}
	if got := v.Names(); len(got) != 4 || got[0] != "CLAYER" {
		t.Fatalf("Names = %v", got)
	}
}

func TestVarsDuplicatePanics(t *testing.T) {
	v := cmdsys.NewVars()
	v.Register(cmdsys.Var{Name: "A", Kind: cmdsys.VarInt, Default: 0})
	defer func() {
		if recover() == nil {
			t.Fatal("registering a name twice must panic")
		}
	}()
	v.Register(cmdsys.Var{Name: "a", Kind: cmdsys.VarInt, Default: 0})
}

func TestSessionLiveVars(t *testing.T) {
	s := newSession()
	if s.Vars().Int("ORTHOMODE") != 0 || s.Vars().Int("APERTURE") != 8 || s.Vars().Text("CLAYER") != "0" {
		t.Fatal("live variables must start at the session's own state")
	}
	s.Ortho = true
	if s.Vars().Int("ORTHOMODE") != 1 {
		t.Fatal("ORTHOMODE must follow Session.Ortho")
	}
	if err := s.Vars().Set("ORTHOMODE", 0); err != nil || s.Ortho {
		t.Fatalf("Set ORTHOMODE 0: %v, Ortho %v", err, s.Ortho)
	}
	if err := s.Vars().Set("APERTURE", 12); err != nil || s.Aperture != 12 {
		t.Fatalf("Set APERTURE: %v, %v", err, s.Aperture)
	}
	s.Drawing().AddLayer("WALLS", 1)
	_ = s.Vars().Set("CLAYER", "WALLS")
	if s.Drawing().CurrentLayer != "WALLS" {
		t.Fatal("CLAYER must set the current layer")
	}
	_ = s.Vars().Set("CLAYER", "NOSUCH")
	if s.Drawing().CurrentLayer != "WALLS" {
		t.Fatal("CLAYER must not switch to a missing layer")
	}
	var heard string
	s.OnVarChange(func(name string, _, _ any) { heard = name })
	_ = s.Vars().Set("LUPREC", 2)
	if heard != "LUPREC" {
		t.Fatalf("OnVarChange heard %q", heard)
	}
}

func TestLastPointVar(t *testing.T) {
	s := newSession()
	s.Script("LINE 2,3 5,5 ")
	if got := s.Vars().Point("LASTPOINT"); got != geom.V(5, 5) {
		t.Fatalf("LASTPOINT = %v, want the last point entered", got)
	}
	_ = s.Vars().Set("LASTPOINT", geom.V(1, 1))
	s.Script("LINE @1,0 @1,0 ")
	hs := s.Drawing().Handles()
	l := s.Drawing().Entity(hs[len(hs)-1]).(*doc.Line)
	if l.Start != geom.V(2, 1) || l.End != geom.V(3, 1) {
		t.Fatalf("@ measured from %v..%v, want 2,1..3,1 (relative to the LASTPOINT 1,1)", l.Start, l.End)
	}
}
