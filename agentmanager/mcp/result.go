package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Content is one result item; Type is "text" or "image".
type Content struct {
	Type     string
	Text     string
	Data     string
	MIMEType string
}

// Result is a tools/call result.
type Result struct {
	Content    []Content
	IsError    bool
	Structured any
}

// Text returns a result with one text content.
func Text(s string) Result {
	return Result{Content: []Content{{Type: "text", Text: s}}}
}

// JSON returns a result with one text content holding compact JSON of v.
func JSON(v any) Result {
	b, err := json.Marshal(v)
	if err != nil {
		return Errorf("encode result: %v", err)
	}
	return Text(string(b))
}

// Image returns a base64 image/png content.
func Image(png []byte) Content {
	return Content{Type: "image", Data: base64.StdEncoding.EncodeToString(png), MIMEType: "image/png"}
}

// Errorf returns an IsError result with the formatted message.
func Errorf(format string, a ...any) Result {
	r := Text(fmt.Sprintf(format, a...))
	r.IsError = true
	return r
}

type wireContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
}

type wireResult struct {
	Content           []wireContent `json:"content"`
	IsError           bool          `json:"isError"`
	StructuredContent any           `json:"structuredContent,omitempty"`
}

func (r Result) wire() wireResult {
	w := wireResult{Content: make([]wireContent, 0, len(r.Content)), IsError: r.IsError, StructuredContent: r.Structured}
	for _, c := range r.Content {
		t := c.Type
		if t == "" {
			t = "text"
		}
		w.Content = append(w.Content, wireContent{Type: t, Text: c.Text, Data: c.Data, MIMEType: c.MIMEType})
	}
	return w
}
