package main

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"time"

	"cad-lite/analytics"
	"cad-lite/appenv"
)

// Optional project ingestion values are supplied by desktop build scripts.
var posthogURLBase64, posthogKeyBase64 string

func startAnalytics() *analytics.Client {
	if appenv.Get("ANALYTICS") == "off" {
		return nil
	}
	endpoint, urlErr := base64.StdEncoding.DecodeString(posthogURLBase64)
	key, keyErr := base64.StdEncoding.DecodeString(posthogKeyBase64)
	posthog := analytics.PostHog{URL: string(endpoint), Key: string(key)}
	var sink analytics.Sink = analytics.Stdout{Writer: os.Stdout}
	if appenv.Get("ANALYTICS") != "stdout" && urlErr == nil && keyErr == nil && posthog.Configured() {
		sink = posthog
	}
	client := analytics.New(sink, version)
	analytics.Record(client, "app_started", nil)
	return client
}

func stopAnalytics(client *analytics.Client) {
	analytics.Record(client, "app_stopped", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = client.Close(ctx)
}

func startupDrawingFormat(path string) string {
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".dwg"):
		return "dwg"
	case strings.HasSuffix(strings.ToLower(path), ".dxf"):
		return "dxf"
	default:
		return "other"
	}
}
