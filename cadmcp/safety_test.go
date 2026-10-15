package cadmcp_test

import (
	"fmt"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

type recFiles struct{ opens, saves int }

func (f *recFiles) Open(string) (*doc.Drawing, []string, error) {
	f.opens++
	return doc.New(), nil, nil
}
func (f *recFiles) Save(string, *doc.Drawing) ([]string, error) { f.saves++; return nil, nil }

func TestScriptsCannotReachFileOrDestructiveCommands(t *testing.T) {
	r := newRig(t)
	rf := &recFiles{}
	r.s.SetFiles(rf)
	r.must("add_line", `{"x1":0,"y1":0,"x2":5,"y2":5}`)
	cases := []struct{ tool, args string }{
		{"run_script", `{"script":"NEW"}`},
		{"run_script", `{"script":"OPEN x.dxf"}`},
		{"run_script", `{"script":"SAVE x.dxf"}`},
		{"run_script", `{"script":"save x.dxf"}`},
		{"run_command", `{"input":"NEW"}`},
		{"run_command", `{"input":"SAVE"}`},
		{"run_script", `{"script":"TEXTEDIT"}`},
		{"run_command", `{"input":"DDEDIT"}`},
	}
	for _, c := range cases {
		m := r.must(c.tool, c.args)
		out, _ := m["output"].([]any)
		if !strings.Contains(toText(out), "not available to agents") {
			t.Errorf("%s %s: output %v", c.tool, c.args, out)
		}
		if got := r.s.Drawing().Len(); got != 1 {
			t.Fatalf("%s %s: drawing has %d entities, want 1", c.tool, c.args, got)
		}
	}
	if rf.opens != 0 || rf.saves != 0 {
		t.Fatalf("file handler reached: %+v", rf)
	}
	if m := r.must("get_drawing", `{}`); m["can_undo"] != true {
		t.Fatalf("undo journal lost: %v", m)
	}
	// A human session still can.
	r.s.Run("SAVE")
	r.s.Submit("x.dxf")
	if rf.saves != 1 {
		t.Fatalf("human SAVE blocked: %+v", rf)
	}
	r.s.Run("NEW")
	if r.s.Drawing().Len() != 0 {
		t.Fatal("human NEW blocked")
	}
}

func TestAgentCannotAnswerHumanFilePrompt(t *testing.T) {
	r := newRig(t)
	rf := &recFiles{}
	r.s.SetFiles(rf)
	r.s.Run("SAVE") // human starts SAVE, prompt waits for a path
	m := r.must("run_command", `{"input":"/home/me/precious.dxf"}`)
	if rf.saves != 0 {
		t.Fatalf("agent answered the human's SAVE prompt: %v", m)
	}
}

func toText(out []any) string {
	var b strings.Builder
	for _, o := range out {
		s, _ := o.(string)
		b.WriteString(s + "\n")
	}
	return b.String()
}

type boomCmd struct{}

func (boomCmd) Name() string { return "BOOM" }
func (boomCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	ctx.Tx().Add(&doc.Line{End: geom.V(1, 1)})
	panic("boom")
}
func (boomCmd) Input(*cmdsys.Context, cmdsys.Input) *cmdsys.Prompt      { return nil }
func (boomCmd) Preview(*cmdsys.Context, geom.Vec2, *render.DisplayList) {}
func (boomCmd) Cancel(*cmdsys.Context)                                  {}

func TestPanickingCommandDoesNotBrickDrawing(t *testing.T) {
	r := newRig(t)
	r.s.Registry().Register("BOOM", func() cmdsys.Command { return boomCmd{} })
	r.must("run_script", `{"script":"BOOM"}`)
	r.must("add_line", `{"x1":0,"y1":0,"x2":5,"y2":5}`)
	if got := r.s.Drawing().Len(); got != 1 {
		t.Fatalf("entities = %d, want 1 (panicked command's work aborted)", got)
	}
}

func TestInsertLibraryCannotReadFilesForAgents(t *testing.T) {
	r := newRig(t)
	files := &recFiles{}
	r.s.SetFiles(files)
	for _, script := range []string{"INSERT secret.dxf", "-INSERT secret.dxf", "I secret.dxf", "SCRIPT\nINSERT secret.dxf\n", "WBLOCK"} {
		r.must("run_script", fmt.Sprintf(`{"script":%q}`, script))
		r.s.Escape()
	}
	r.s.Run("INSERT")
	r.must("run_command", `{"input":"secret.dxf"}`)
	r.s.Escape()
	if files.opens != 0 || files.saves != 0 {
		t.Fatalf("agent reached filesystem: %+v", files)
	}
	tx, _ := r.s.Drawing().Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "safe", Entities: []doc.Entity{&doc.Circle{Radius: 1}}}); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	r.must("run_script", `{"script":"INSERT\nsafe\n0,0\n1\n0\n"}`)
	if r.s.Drawing().Len() != 1 {
		t.Fatal("named INSERT should remain agent-safe", r.s.History())
	}
}
