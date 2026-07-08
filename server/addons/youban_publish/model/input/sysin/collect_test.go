package sysin

import "testing"

func TestNormalizeCollectHistoryConfig(t *testing.T) {
	tests := []struct {
		name        string
		sourceType  string
		enabled     int
		mode        string
		days        int
		wantEnabled int
		wantMode    string
		wantDays    int
	}{
		{
			name:        "account enabled defaults",
			sourceType:  CollectSourceTypeAccount,
			enabled:     1,
			wantEnabled: 1,
			wantMode:    CollectHistoryModeRecentDays,
			wantDays:    30,
		},
		{
			name:        "account clamps max days",
			sourceType:  CollectSourceTypeAccount,
			enabled:     1,
			mode:        "unknown",
			days:        400,
			wantEnabled: 1,
			wantMode:    CollectHistoryModeRecentDays,
			wantDays:    365,
		},
		{
			name:        "account all history",
			sourceType:  CollectSourceTypeAccount,
			enabled:     1,
			mode:        CollectHistoryModeAll,
			days:        30,
			wantEnabled: 1,
			wantMode:    CollectHistoryModeAll,
			wantDays:    30,
		},
		{
			name:        "bot disabled",
			sourceType:  CollectSourceTypeBot,
			enabled:     1,
			mode:        CollectHistoryModeRecentDays,
			days:        7,
			wantEnabled: 0,
			wantMode:    CollectHistoryModeRecentDays,
			wantDays:    7,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEnabled, gotMode, gotDays := NormalizeCollectHistoryConfig(tt.sourceType, tt.enabled, tt.mode, tt.days)
			if gotEnabled != tt.wantEnabled || gotMode != tt.wantMode || gotDays != tt.wantDays {
				t.Fatalf("NormalizeCollectHistoryConfig() = (%d,%q,%d), want (%d,%q,%d)",
					gotEnabled, gotMode, gotDays, tt.wantEnabled, tt.wantMode, tt.wantDays)
			}
		})
	}
}
