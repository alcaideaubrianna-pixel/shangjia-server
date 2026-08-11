package sys

import "testing"

func TestDefaultAccountSettingDisablesTitleMark(t *testing.T) {
	setting := defaultAccountSetting(123)
	if setting.AccountId != 123 {
		t.Fatalf("accountId = %d, want 123", setting.AccountId)
	}
	if setting.EnableTitleMark != 0 {
		t.Fatalf("enableTitleMark = %d, want 0", setting.EnableTitleMark)
	}
}
