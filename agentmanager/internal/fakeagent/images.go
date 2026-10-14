package main

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// imageNote describes one received image for the echo builtins: " [image <mime> <n> bytes]", n
// being the decoded size, or " [image <mime> bad base64]".
func imageNote(mime, data string) string {
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return fmt.Sprintf(" [image %s bad base64]", mime)
	}
	return fmt.Sprintf(" [image %s %d bytes]", mime, len(b))
}

// dataURLNote is imageNote for a data:<mime>;base64,<data> URL (Codex image items).
func dataURLNote(url string) string {
	rest, ok := strings.CutPrefix(url, "data:")
	mime, data, ok2 := strings.Cut(rest, ";base64,")
	if !ok || !ok2 {
		return " [image bad url]"
	}
	return imageNote(mime, data)
}
