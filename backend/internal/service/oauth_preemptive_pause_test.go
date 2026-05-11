package service

import (
	"testing"
	"time"
)

func TestAccountOAuthOfficialWindowState_OpenAI7d(t *testing.T) {
	now := time.Date(2026, 5, 9, 21, 0, 0, 0, time.UTC)
	resetAt := now.Add(24 * time.Hour)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_7d_used_percent": 88.0,
			"codex_7d_reset_at":     resetAt.Format(time.RFC3339),
		},
	}

	util, start, reset, ok := accountOAuthOfficialWindowState(account, "7d", now)
	if !ok {
		t.Fatal("expected OpenAI 7d window to be available")
	}
	if util != 88.0 {
		t.Fatalf("utilization = %v, want 88", util)
	}
	if !reset.Equal(resetAt) {
		t.Fatalf("resetAt = %v, want %v", reset, resetAt)
	}
	if !start.Equal(resetAt.Add(-7 * 24 * time.Hour)) {
		t.Fatalf("startTime = %v, want %v", start, resetAt.Add(-7*24*time.Hour))
	}
}

func TestAccountOAuthOfficialWindowState_Anthropic5h(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	windowEnd := now.Add(2 * time.Hour)
	windowStart := now.Add(-3 * time.Hour)
	account := &Account{
		Platform:           PlatformAnthropic,
		Type:               AccountTypeOAuth,
		SessionWindowStart: &windowStart,
		SessionWindowEnd:   &windowEnd,
		Extra: map[string]any{
			"session_window_utilization": 0.42,
		},
	}

	util, start, reset, ok := accountOAuthOfficialWindowState(account, "5h", now)
	if !ok {
		t.Fatal("expected Anthropic 5h window to be available")
	}
	if util != 42.0 {
		t.Fatalf("utilization = %v, want 42", util)
	}
	if !reset.Equal(windowEnd) {
		t.Fatalf("resetAt = %v, want %v", reset, windowEnd)
	}
	if !start.Equal(windowStart) {
		t.Fatalf("startTime = %v, want %v", start, windowStart)
	}
}
