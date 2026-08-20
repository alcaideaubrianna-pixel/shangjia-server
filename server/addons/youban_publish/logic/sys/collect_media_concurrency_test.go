package sys

import "testing"

func TestNormalizeCollectMediaConcurrency(t *testing.T) {
	tests := []struct {
		name                    string
		global, account         int
		wantGlobal, wantAccount int
	}{
		{name: "defaults", global: collectMediaDefaultGlobalConcurrency, account: collectMediaDefaultAccountConcurrency, wantGlobal: 64, wantAccount: 8},
		{name: "minimum", global: 0, account: 0, wantGlobal: 1, wantAccount: 1},
		{name: "maximum", global: 1000, account: 100, wantGlobal: collectMediaMaxGlobalConcurrency, wantAccount: collectMediaMaxAccountConcurrency},
		{name: "account bounded by global", global: 2, account: 8, wantGlobal: 2, wantAccount: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			globalLimit, accountLimit := normalizeCollectMediaConcurrency(test.global, test.account)
			if globalLimit != test.wantGlobal || accountLimit != test.wantAccount {
				t.Fatalf("normalizeCollectMediaConcurrency(%d,%d)=(%d,%d) want=(%d,%d)", test.global, test.account, globalLimit, accountLimit, test.wantGlobal, test.wantAccount)
			}
		})
	}
}
