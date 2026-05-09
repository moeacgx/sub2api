package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestApplyResolvedOAuthPauseConfig_UsesStrictestGroupFallback(t *testing.T) {
	account := &service.Account{
		Extra:  map[string]any{},
		Groups: []*service.Group{
			{
				ID:                  1,
				OAuth5hPausePercent: ptrFloat64(80),
				OAuth5hPauseAmount:  ptrFloat64(25),
				OAuth7dPausePercent: ptrFloat64(90),
				OAuth7dPauseAmount:  ptrFloat64(100),
			},
			{
				ID:                  2,
				OAuth5hPausePercent: ptrFloat64(70),
				OAuth5hPauseAmount:  ptrFloat64(20),
				OAuth7dPausePercent: ptrFloat64(85),
				OAuth7dPauseAmount:  ptrFloat64(80),
			},
		},
	}

	applyResolvedOAuthPauseConfig(account)

	if got := account.Extra["effective_oauth_5h_pause_percent"]; got != 70.0 {
		t.Fatalf("effective_oauth_5h_pause_percent = %v, want 70", got)
	}
	if got := account.Extra["effective_oauth_5h_pause_amount_usd"]; got != 20.0 {
		t.Fatalf("effective_oauth_5h_pause_amount_usd = %v, want 20", got)
	}
	if got := account.Extra["effective_oauth_7d_pause_percent"]; got != 85.0 {
		t.Fatalf("effective_oauth_7d_pause_percent = %v, want 85", got)
	}
	if got := account.Extra["effective_oauth_7d_pause_amount_usd"]; got != 80.0 {
		t.Fatalf("effective_oauth_7d_pause_amount_usd = %v, want 80", got)
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}
