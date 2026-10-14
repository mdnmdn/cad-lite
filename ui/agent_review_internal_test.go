package ui

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"agentmanager"
)

// Regressions from the agent-side review (internal parts).

// M1: pathological Markdown parses in bounded time.
func TestMarkdownPathologicalInputIsBounded(t *testing.T) {
	nested := func() string {
		var b strings.Builder
		for i := 0; i < 300; i++ {
			b.WriteString(strings.Repeat(" ", i*2) + "- x\n")
		}
		return b.String()
	}()
	ticks := func() string {
		var b strings.Builder
		for i := 1; i < 200; i++ {
			b.WriteString(strings.Repeat("`", i) + "x ")
		}
		return b.String()
	}()
	cases := map[string]string{
		"normal50k":   strings.Repeat("Some **bold** text with `code` and a [link](https://x.y/z).\n\n- item one\n- item two\n\n", 600),
		"starOpen":    strings.Repeat("*a ", 7000),
		"underscores": strings.Repeat("_a ", 7000),
		"tildes":      strings.Repeat("~~a ", 5000),
		"doubleStar":  strings.Repeat("**a ", 5000),
		"brackets":    strings.Repeat("[", 200000),
		"autolinks":   strings.Repeat("<", 200000),
		"escapes":     strings.Repeat("\\*\n", 20000),
		"links":       strings.Repeat("[a](", 5000),
		"ticks":       ticks,
		"quotes":      strings.Repeat(">", 5000) + " hi",
		"lists":       strings.Repeat("- ", 2000) + "x",
		"ordered":     strings.Repeat("1. ", 2000) + "x",
		"nestedList":  nested,
		"emphNest":    strings.Repeat("*_", 3000) + "x",
		"longLine":    strings.Repeat("abcdefghij ", 20000),
	}
	for name, s := range cases {
		start := time.Now()
		parseMarkdown(s)
		if d := time.Since(start); d > 50*time.Millisecond {
			t.Errorf("%s (%d bytes) took %v", name, len(s), d)
		}
	}
}

// M1: nesting deeper than mdMaxDepth becomes text instead of recursing further.
func TestMarkdownNestingIsCapped(t *testing.T) {
	bs := parseMarkdown(strings.Repeat("> ", 40) + "deep")
	depth := 0
	for len(bs) == 1 && bs[0].kind == mdQuote {
		depth++
		bs = bs[0].sub
	}
	if depth > mdMaxDepth || len(bs) != 1 || bs[0].kind != mdPara || !strings.HasSuffix(mdPlain(bs[0].inl), "deep") {
		t.Fatalf("depth %d, rest %+v", depth, bs)
	}
}

// M1: while a turn runs, a streaming message is re-parsed at most agentParseEvery.
func TestAgentStreamingParseIsThrottled(t *testing.T) {
	r := newLongTranscriptRig(t, 1)
	tab := r.p.cur
	tab.tr.AddUser("go")
	tab.tr.Apply(agentmanager.TextDelta{MessageID: "s", Text: "a"})
	i := len(tab.tr.Blocks()) - 1
	r.frame()
	first := tab.md[i]
	tab.tr.Apply(agentmanager.TextDelta{MessageID: "s", Text: " **b**"})
	r.frame()
	if tab.md[i].rev != first.rev {
		t.Fatal("re-parsed within the throttle period while streaming")
	}
	if err := r.plat.Advance(agentParseEvery + time.Millisecond); err != nil {
		t.Fatal(err)
	}
	r.frame()
	if tab.md[i].rev != tab.tr.Revision(i) {
		t.Fatal("not re-parsed after the throttle period")
	}
	// Once the turn is over the parse catches up at once.
	tab.tr.Apply(agentmanager.TextDelta{MessageID: "s", Text: " c"})
	tab.tr.Apply(agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn})
	r.frame()
	if tab.md[i].rev != tab.tr.Revision(i) {
		t.Fatal("finished message left with a stale parse")
	}
}

// L5: URLs with bidi controls are not links; balanced parentheses stay in bare URLs.
func TestMarkdownURLSafety(t *testing.T) {
	for _, s := range []string{"[x](https://a.b/‮gpj.exe)", "<https://a.b/⁦x>", "see https://a.b/‮x"} {
		for _, sp := range parseInline(s) {
			if sp.link != "" {
				t.Errorf("%q made a link to %q", s, sp.link)
			}
		}
	}
	links := func(s string) []string {
		var out []string
		for _, sp := range parseInline(s) {
			if sp.link != "" {
				out = append(out, sp.link)
			}
		}
		return out
	}
	if got := links("see https://en.wikipedia.org/wiki/Foo_(bar) now"); len(got) != 1 || got[0] != "https://en.wikipedia.org/wiki/Foo_(bar)" {
		t.Errorf("balanced parens: %q", got)
	}
	if got := links("(see https://a.b/c)."); len(got) != 1 || got[0] != "https://a.b/c" {
		t.Errorf("enclosing parens: %q", got)
	}
	var opened []string
	p := &AgentPanel{openURL: func(u string) error { opened = append(opened, u); return nil }}
	p.agentSet = &agentSet{cur: newAgentTab()}
	p.openLink("https://a.b/‮x")
	if len(opened) != 0 {
		t.Fatalf("opened %q", opened)
	}
}

// L6: an image whose header announces too many pixels is refused before decoding.
func TestAgentImageRefusesHugeDimensions(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	// Patch the IHDR width and height to 10000 x 10000 (100 MP) and fix the chunk's CRC: the
	// file is tiny but announces 100 MP.
	copy(data[16:24], []byte{0, 0, 0x27, 0x10, 0, 0, 0x27, 0x10})
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, err := newAgentImage("big.png", data); !errors.Is(err, errImagePixels) {
		t.Fatalf("err = %v, want errImagePixels", err)
	}
}

// L7: concurrent clipboard writes end with the last click's text.
func TestAgentClipboardLastClickWins(t *testing.T) {
	r := newLongTranscriptRig(t, 1)
	var mu sync.Mutex
	var final string
	slow := make(chan struct{})
	calls := 0
	r.p.clip = func(s string) error {
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		if first {
			<-slow // the first write is still running when the second click comes
		}
		mu.Lock()
		final = s
		mu.Unlock()
		return nil
	}
	w := r.hw.Window()
	r.p.copyText(r.p.cur, 1, "first", w)
	time.Sleep(5 * time.Millisecond)
	r.p.copyText(r.p.cur, 2, "second", w)
	time.Sleep(5 * time.Millisecond)
	close(slow)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		f := final
		mu.Unlock()
		if f == "second" && r.p.clipIdle() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("clipboard ends with %q", final)
}

// quietSession is a session that starts and then says nothing.
type quietSession struct {
	events    chan agentmanager.Event
	promptErr error
	prompts   int
}

func (s *quietSession) ID() string                                                    { return "quiet" }
func (s *quietSession) Events() <-chan agentmanager.Event                             { return s.events }
func (s *quietSession) Prompt(context.Context, string) error                          { s.prompts++; return s.promptErr }
func (s *quietSession) Cancel() error                                                 { return nil }
func (s *quietSession) Close() error                                                  { return nil }
func (s *quietSession) RespondPermission(string, agentmanager.PermissionAnswer) error { return nil }

// M2: a turn with no events redraws the window about agentWorkingFPS times a second, not 60.
func TestAgentIdleTurnRedrawRate(t *testing.T) {
	r := newLongTranscriptRig(t, 1)
	s := &quietSession{events: make(chan agentmanager.Event, 4)}
	r.p.SetOpener(func(context.Context, agentmanager.Config) (agentmanager.Session, error) { return s, nil })
	r.p.cur.tr.Apply(agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}) // the seeded turn
	r.p.Send("go", r.hw.Window())
	for i := 0; i < 200 && r.p.cur.state != agentReady; i++ {
		_ = r.plat.Advance(16 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}
	if r.p.cur.state != agentReady || !r.p.cur.tr.Running() {
		t.Fatal("turn not running")
	}
	_ = r.plat.Advance(time.Second) // settle
	before := r.p.frames
	for i := 0; i < 1000; i++ {
		_ = r.plat.Advance(time.Millisecond)
	}
	got := r.p.frames - before
	t.Logf("idle turn: %d redraws per second", got)
	if got > 2*agentWorkingFPS || got < agentWorkingFPS/2 {
		t.Fatalf("idle turn redraws %d times a second, want about %d", got, agentWorkingFPS)
	}
}

func TestAgentFailedQueueDrainsInBoundedBatches(t *testing.T) {
	r := newLongTranscriptRig(t, 1)
	tab := r.p.cur
	tab.tr.Apply(agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn})
	sess := &quietSession{events: make(chan agentmanager.Event), promptErr: errors.New("failed")}
	tab.sess, tab.state = sess, agentReady
	tab.queue = make([]agentQueued, agentEventBatch+2)
	for i := range tab.queue {
		tab.queue[i].text = "prompt"
	}
	r.p.nextQueued(tab)
	if sess.prompts != agentEventBatch || len(tab.queue) != 2 {
		t.Fatalf("drained %d prompts, remaining %d; want bounded batch and 2 remaining", sess.prompts, len(tab.queue))
	}
	r.p.pump(r.hw.Window())
	if len(tab.queue) != 0 {
		t.Fatalf("remaining queue: %d", len(tab.queue))
	}
}

func TestAgentModelRestartPreservesPausedQueue(t *testing.T) {
	r := newLongTranscriptRig(t, 1)
	tab := r.p.cur
	sess := &quietSession{events: make(chan agentmanager.Event, 1)}
	tab.sess, tab.state = sess, agentReady
	tab.queue = []agentQueued{{text: "later"}}
	tab.queuePaused, tab.mdl.restart = true, true
	sess.events <- agentmanager.TurnEnd{Reason: agentmanager.StopCancelled}
	r.p.drainEvents(tab)
	if !tab.queuePaused || len(tab.queue) != 1 || tab.state != agentIdle {
		t.Fatalf("pause %v, queue %d, state %v", tab.queuePaused, len(tab.queue), tab.state)
	}
}

func BenchmarkMarkdownUnclosedMarkers(b *testing.B) {
	for _, marker := range []string{"[", "<", "*a "} {
		b.Run(marker, func(b *testing.B) {
			text := strings.Repeat(marker, 200000/len(marker))
			b.SetBytes(int64(len(text)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				parseMarkdown(text)
			}
		})
	}
}
