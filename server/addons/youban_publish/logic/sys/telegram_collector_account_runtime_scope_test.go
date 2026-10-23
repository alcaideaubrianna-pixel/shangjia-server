package sys

import "testing"

func TestAccountRuntimeScopeAllows(t *testing.T) {
	t.Setenv("YOUBAN_TELEGRAM_ACCOUNT_INCLUDE_IDS", "")
	t.Setenv("YOUBAN_TELEGRAM_ACCOUNT_EXCLUDE_IDS", "")
	if !accountRuntimeScopeAllows(67) {
		t.Fatal("default scope must preserve all authorized accounts")
	}

	t.Setenv("YOUBAN_TELEGRAM_ACCOUNT_EXCLUDE_IDS", "67, 68")
	if accountRuntimeScopeAllows(67) || !accountRuntimeScopeAllows(69) {
		t.Fatal("exclude scope did not isolate the selected accounts")
	}

	t.Setenv("YOUBAN_TELEGRAM_ACCOUNT_INCLUDE_IDS", "67; invalid")
	if !accountRuntimeScopeAllows(67) || accountRuntimeScopeAllows(68) {
		t.Fatal("include scope did not restrict the runtime to selected accounts")
	}
}

func TestParseAccountRuntimeScope(t *testing.T) {
	got := parseAccountRuntimeScope("67, 68;69\n70 bad -1 0")
	for _, id := range []int64{67, 68, 69, 70} {
		if _, ok := got[id]; !ok {
			t.Fatalf("account %d missing from parsed scope", id)
		}
	}
	if len(got) != 4 {
		t.Fatalf("unexpected parsed scope: %+v", got)
	}
}
