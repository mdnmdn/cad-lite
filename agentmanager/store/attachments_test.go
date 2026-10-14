package store_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agentmanager"
	"agentmanager/store"
	"agentmanager/transcript"
)

func TestUserAttachmentsRoundTrip(t *testing.T) {
	s := open(t)
	r, err := s.Create(agentmanager.OpenCode)
	if err != nil {
		t.Fatal(err)
	}
	att := []transcript.Attachment{{Name: "plan.png", MIME: "image/png", Width: 640, Height: 480, Thumb: []byte{1, 2, 3}},
		{MIME: "image/jpeg", Width: 10, Height: 20}}
	if err := r.AddUserWith("look at this", att); err != nil {
		t.Fatal(err)
	}
	if err := r.AddUser("plain"); err != nil {
		t.Fatal(err)
	}
	r.Close()
	b, err := os.ReadFile(filepath.Join(s.Dir(), r.Meta().ID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"d":{"Text":"plain"}`) {
		t.Errorf("a message without attachments changed its stored form:\n%s", b)
	}
	loaded, err := s.Load(r.Meta().ID)
	if err != nil {
		t.Fatal(err)
	}
	if m := loaded.Meta(); m.Title != "look at this" {
		t.Errorf("title %q", m.Title)
	}
	var tr transcript.T
	loaded.Replay(&tr)
	want := []transcript.Block{transcript.UserMsg{Text: "look at this", Attachments: att}, transcript.UserMsg{Text: "plain"}}
	if !reflect.DeepEqual(tr.Blocks()[:2], want) {
		t.Fatalf("replayed %#v", tr.Blocks())
	}
	if items := loaded.Items(); !reflect.DeepEqual(items[0], store.User{Text: "look at this", Attachments: att}) {
		t.Fatalf("item %#v", items[0])
	}
}
