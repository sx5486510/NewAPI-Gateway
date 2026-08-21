package model

import "testing"

func TestEvaluateClientRestrictionTruthTable(t *testing.T) {
	tests := []struct {
		name         string
		allowCodex   bool
		allowCC      bool
		blockClients bool
		wantCodex    bool
		wantCC       bool
		wantGeneric  bool
	}{
		{
			name:      "no restriction allows everyone",
			wantCodex: true, wantCC: true, wantGeneric: true,
		},
		{
			name:       "codex only reserves route for codex",
			allowCodex: true,
			wantCodex:  true, wantCC: false, wantGeneric: false,
		},
		{
			name:      "cc only reserves route for cc",
			allowCC:   true,
			wantCodex: false, wantCC: true, wantGeneric: false,
		},
		{
			name:       "codex and cc excludes generic",
			allowCodex: true, allowCC: true,
			wantCodex: true, wantCC: true, wantGeneric: false,
		},
		{
			name:         "block clients rejects identified clients only",
			blockClients: true,
			wantCodex:    false, wantCC: false, wantGeneric: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluateClientRestriction("codex", tt.allowCodex, tt.allowCC, tt.blockClients); got != tt.wantCodex {
				t.Errorf("codex: got %v, want %v", got, tt.wantCodex)
			}
			if got := EvaluateClientRestriction("cc", tt.allowCodex, tt.allowCC, tt.blockClients); got != tt.wantCC {
				t.Errorf("cc: got %v, want %v", got, tt.wantCC)
			}
			if got := EvaluateClientRestriction("", tt.allowCodex, tt.allowCC, tt.blockClients); got != tt.wantGeneric {
				t.Errorf("generic: got %v, want %v", got, tt.wantGeneric)
			}
		})
	}
}

func TestEvaluateClientRestrictionNormalizesConflictingFlags(t *testing.T) {
	// blockClients 与 allow* 同时为真时，blockClients 优先（与
	// NormalizeClientRestrictions 一致），因此 codex 被拒、泛客户端放行。
	if EvaluateClientRestriction("codex", true, true, true) {
		t.Error("expected blockClients to win over allow flags for codex")
	}
	if !EvaluateClientRestriction("", true, true, true) {
		t.Error("expected blockClients to allow generic client")
	}
}

func TestEvaluateClientRestrictionRejectsUnknownClientType(t *testing.T) {
	// 未来新增客户端类型时默认拒绝受限线路，避免静默放量。
	if EvaluateClientRestriction("gemini-cli", true, false, false) {
		t.Error("expected unknown client type to be rejected on a restricted route")
	}
	if !EvaluateClientRestriction("gemini-cli", false, false, false) {
		t.Error("expected unknown client type to pass an unrestricted route")
	}
}
