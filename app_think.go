//go:build windows

package main

import (
	"context"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// thinkApplyBody merges thinking configuration into the request body map.
// effort="" means disabled; "high"/"max" map to the appropriate API fields.
func thinkApplyBody(bodyMap map[string]any, apiFormat, effort string) {
	if effort == "" {
		return
	}
	switch apiFormat {
	case "anthropic":
		bt := 4000
		if effort == "max" {
			bt = 16000
		}
		bodyMap["thinking"] = map[string]any{"type": "enabled", "budget_tokens": bt}
		bodyMap["output_config"] = map[string]string{"effort": effort}
	default: // OpenAI (DeepSeek, Qwen, etc.)
		bodyMap["thinking"] = map[string]string{"type": "enabled"}
		bodyMap["reasoning_effort"] = effort
	}
}

// thinkSSEDelta handles a content_block_delta event for thinking/signature.
// Returns true if it consumed the event (caller should skip further processing).
func thinkSSEDelta(ctx context.Context, eventName string, delta map[string]any) bool {
	dt, _ := delta["type"].(string)
	switch dt {
	case "thinking_delta":
		if text, _ := delta["thinking"].(string); text != "" {
			wailsRuntime.EventsEmit(ctx, eventName, map[string]any{"reasoning": text})
		}
		return true
	case "signature_delta":
		return true // no-op, accepted for multi-turn passback
	}
	return false
}
