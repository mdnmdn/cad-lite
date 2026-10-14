package mcp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"agentmanager/mcp"
)

// G49 review: the handler's context must carry the deadline by which its reply still fits in the
// write timeout, counted from the request's arrival (not from the end of a slow body).
func TestHandlerContextHasReplyDeadline(t *testing.T) {
	const wt = 3 * time.Second
	defer mcp.SetWriteTimeout(wt)()
	got := make(chan time.Time, 1)
	s := mcp.NewServer(mcp.Options{})
	s.Register(mcp.Tool{Name: "dl", Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
		d, ok := ctx.Deadline()
		if !ok {
			d = time.Time{}
		}
		got <- d
		return mcp.Text("ok"), nil
	}})
	ep, _ := s.Start()
	defer s.Close()
	pr, pw := io.Pipe()
	req, _ := http.NewRequest("POST", ep.URL, pr)
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	sent := time.Now()
	go func() {
		_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call",`))
		time.Sleep(400 * time.Millisecond)
		_, _ = pw.Write([]byte(`"params":{"name":"dl"}}`))
		pw.Close()
	}()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	d := <-got
	if d.IsZero() {
		t.Fatal("handler context has no deadline")
	}
	if left := d.Sub(sent); left > wt-mcp.ReplyMargin(wt)+100*time.Millisecond || left < wt-mcp.ReplyMargin(wt)-300*time.Millisecond {
		t.Fatalf("deadline %v after the request was sent, want about %v", left, wt-mcp.ReplyMargin(wt))
	}
}
