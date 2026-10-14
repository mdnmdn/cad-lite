package mcp

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
func Text(s string) Result { panic("todo: WP2") }

// JSON returns a result with one text content holding compact JSON of v.
func JSON(v any) Result { panic("todo: WP2") }

// Image returns a base64 image/png content.
func Image(png []byte) Content { panic("todo: WP2") }

// Errorf returns an IsError result with the formatted message.
func Errorf(format string, a ...any) Result { panic("todo: WP2") }
