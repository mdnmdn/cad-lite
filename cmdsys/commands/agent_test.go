package commands_test

import (
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/render"
)

func TestAICommandCallsHandler(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	s.Submit("ai")
	if s.Active() != "" {
		t.Fatal("AI without an agent handler should end at once")
	}
	var got []string
	s.SetAgentHandler(func(p string) { got = append(got, p) })
	s.Submit("AGENT")
	if s.Active() != "AI" || s.CurrentPrompt().Accepts&cmdsys.InputText == 0 {
		t.Fatalf("active %q prompt %v", s.Active(), s.CurrentPrompt())
	}
	s.Submit("draw a box")
	s.Submit("ai")
	s.Submit("")
	if len(got) != 2 || got[0] != "draw a box" || got[1] != "" || s.Active() != "" {
		t.Fatalf("handler calls = %q", got)
	}
}

func TestFileCommandsSetFilePath(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	s.SetFiles(nopFiles{})
	var paths []string
	s.OnFilePath(func(p string) { paths = append(paths, p) })
	s.Submit("open")
	s.Submit("/x/a.dxf")
	s.Submit("save")
	s.Submit("/y/b.dxf")
	if len(paths) != 2 || paths[0] != "/x/a.dxf" || s.FilePath() != "/y/b.dxf" {
		t.Fatalf("paths = %v", paths)
	}
}

type nopFiles struct{}

func (nopFiles) Open(string) (*doc.Drawing, []string, error) { return doc.New(), nil, nil }
func (nopFiles) Save(string, *doc.Drawing) ([]string, error) { return nil, nil }
