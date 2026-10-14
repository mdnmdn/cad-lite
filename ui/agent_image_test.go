package ui_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"agentmanager"
	"agentmanager/transcript"
	"cad-lite/internal/gpui"
)

// imageSession is a fakeSession that takes images like a real backend.
type imageSession struct {
	*fakeSession
	accepts bool
	mu      sync.Mutex
	sent    [][]agentmanager.Image
}

func (s *imageSession) AcceptsImages() bool { return s.accepts }
func (s *imageSession) PromptImages(ctx context.Context, text string, imgs []agentmanager.Image) error {
	if len(imgs) > 0 && !s.accepts {
		return agentmanager.ErrNoImages
	}
	s.mu.Lock()
	s.sent = append(s.sent, imgs)
	s.mu.Unlock()
	return s.Prompt(ctx, text)
}

func (s *imageSession) images() [][]agentmanager.Image {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.sent)
}

// withImageSessions makes the rig's first session an imageSession.
func withImageSessions(r *agentRig, accepts bool) *imageSession {
	s := &imageSession{fakeSession: r.fs, accepts: accepts}
	r.ws.AgentPanel().SetOpener(func(_ context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
		r.cfg <- cfg
		return s, nil
	})
	return s
}

// pngFile writes a w x h PNG named name into dir and returns its path.
func pngFile(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 255 / w), G: 120, B: uint8(y * 255 / h), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func (r *agentRig) dropAt(p gpui.Point, paths ...string) {
	r.hw.Dispatch(gpui.FileDropEvent{Position: p, Paths: paths})
	r.hw.Pixels()
}

func TestAgentDropImageOnComposerAttachesAndSends(t *testing.T) {
	r := openAgent(t)
	s := withImageSessions(r, true)
	dir := t.TempDir()
	shot := pngFile(t, dir, "shot.png", 40, 20)
	drawing := filepath.Join(dir, "plan.dxf")
	if err := os.WriteFile(drawing, []byte("0\nEOF\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := r.ws.AgentPanel()
	history := len(r.ws.Session().History())
	r.dropAt(r.inputPos(), shot, drawing)
	if got := p.Attachments(); !slices.Equal(got, []string{"shot.png"}) {
		t.Fatalf("attachments %q", got)
	}
	if n := len(r.ws.Session().History()); n != history {
		t.Fatalf("a drop on the agent panel reached the command line: %q", r.ws.Session().History()[history:])
	}
	if !r.note(0, "plan.dxf") {
		t.Fatal("the dropped drawing was not reported as no image")
	}
	if _, ok := (&tbRig{t: t, hw: r.hw}).boundsAnywhere("agent-image-0"); !ok {
		t.Fatal("no chip for the attached image")
	}

	r.click(r.inputPos())
	r.typ("what is this")
	r.enter()
	r.until("prompt with the image", func() bool { return len(s.images()) == 1 })
	sent := s.images()[0]
	if len(sent) != 1 || sent[0].MIME != "image/png" || !bytes.HasPrefix(sent[0].Data, []byte("\x89PNG")) {
		t.Fatalf("sent %+v", sent)
	}
	if got := s.promptList(); !slices.Equal(got, []string{"what is this"}) {
		t.Fatalf("prompts %q", got)
	}
	if len(p.Attachments()) != 0 {
		t.Fatal("the composer kept the sent image")
	}
	var msg transcript.UserMsg
	for _, b := range p.Transcript().Blocks() {
		if u, ok := b.(transcript.UserMsg); ok {
			msg = u
		}
	}
	if len(msg.Attachments) != 1 || msg.Attachments[0].Name != "shot.png" || msg.Attachments[0].Width != 40 || len(msg.Attachments[0].Thumb) == 0 {
		t.Fatalf("transcript message %+v", msg)
	}
}

func TestAgentDropOutsidePanelStillOpensDrawings(t *testing.T) {
	r := openAgent(t)
	shot := pngFile(t, t.TempDir(), "shot.png", 8, 8)
	history := len(r.ws.Session().History())
	r.dropAt(gpui.Point{X: 200, Y: 400}, shot)
	if len(r.ws.AgentPanel().Attachments()) != 0 {
		t.Fatal("a drop on the canvas was attached to the agent prompt")
	}
	if len(r.ws.Session().History()) == history {
		t.Fatal("a drop on the canvas did not reach the workspace")
	}
}

func TestAgentImageRefusedWithoutVision(t *testing.T) {
	r := openAgent(t)
	withImageSessions(r, false)
	r.start()
	shot := pngFile(t, t.TempDir(), "shot.png", 8, 8)
	p := r.ws.AgentPanel()
	p.AttachFiles([]string{shot}, r.hw.Window())
	if len(p.Attachments()) != 0 {
		t.Fatal("attached to an agent that takes no images")
	}
	if !r.note(0, "does not read images") {
		t.Fatal("no note about the refused image")
	}
}

func TestAgentSendRefusedForTextOnlyModel(t *testing.T) {
	r := openAgent(t)
	s := withImageSessions(r, true)
	r.start()
	shot := pngFile(t, t.TempDir(), "shot.png", 8, 8)
	p, w := r.ws.AgentPanel(), r.hw.Window()
	p.AttachFiles([]string{shot}, w)
	if len(p.Attachments()) != 1 {
		t.Fatal("not attached for a vision model")
	}
	s.events <- agentmanager.ModelsChanged{Current: "tiny", Available: []agentmanager.ModelInfo{{ID: "tiny", Vision: agentmanager.VisionNo}}}
	r.until("models", func() bool { return len(p.Models()) == 1 })
	p.Send("look", w)
	if len(s.promptList()) != 0 {
		t.Fatal("a prompt with images went to a text-only model")
	}
	if len(p.Attachments()) != 1 || !r.note(0, "remove them or pick another model") {
		t.Fatal("the refused prompt lost its images or gave no reason")
	}
	p.RemoveAttachment(0, w)
	p.Send("look", w)
	if got := s.promptList(); !slices.Equal(got, []string{"look"}) {
		t.Fatalf("text-only prompt after removing the image: %q", got)
	}
}

func TestAgentImageChipRemove(t *testing.T) {
	r := openAgent(t)
	dir := t.TempDir()
	a, b := pngFile(t, dir, "a.png", 8, 8), pngFile(t, dir, "b.png", 3000, 1000)
	p := r.ws.AgentPanel()
	p.AttachFiles([]string{a, b}, r.hw.Window())
	r.hw.Pixels()
	if got := p.Attachments(); !slices.Equal(got, []string{"a.png", "b.png"}) {
		t.Fatalf("attachments %q", got)
	}
	r.click(r.pos("agent-image-remove-0"))
	r.hw.Pixels()
	if got := p.Attachments(); !slices.Equal(got, []string{"b.png"}) {
		t.Fatalf("after removing the first: %q", got)
	}
}

func TestAgentPasteImageAttaches(t *testing.T) {
	r := openAgent(t)
	r.click(r.inputPos())
	data, err := os.ReadFile(pngFile(t, t.TempDir(), "x.png", 10, 10))
	if err != nil {
		t.Fatal(err)
	}
	r.hw.Dispatch(gpui.ImagePasteEvent{Data: data, MIME: "image/png"})
	if got := r.ws.AgentPanel().Attachments(); !slices.Equal(got, []string{""}) {
		t.Fatalf("attachments after paste %q", got)
	}
	r.hw.Dispatch(gpui.ImagePasteEvent{Data: []byte("not an image"), MIME: "image/png"})
	if len(r.ws.AgentPanel().Attachments()) != 1 || !r.note(0, "PNG, JPEG, GIF") {
		t.Fatal("a broken paste was attached or not reported")
	}
}
