package sys

import (
	"reflect"
	"testing"

	"hotgo/internal/consts"
)

func TestTGGoTaskLogRetentionTargetsUseSharedPolicyIndexes(t *testing.T) {
	tests := []struct {
		name       string
		database   string
		webhookIdx string
	}{
		{name: "mysql", database: consts.DBMysql, webhookIdx: webhookLogRetentionMysqlIndex},
		{name: "pgsql", database: consts.DBPgsql, webhookIdx: webhookLogRetentionIndex},
		{name: "default mysql", webhookIdx: webhookLogRetentionMysqlIndex},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := tggoTaskLogRetentionTargets(test.database)
			want := []tggoTaskLogRetentionTarget{
				{table: pushLogRetentionTable, index: pushLogRetentionIndex},
				{table: webhookLogRetentionTable, index: test.webhookIdx},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("unexpected TGGo task-log retention targets: got %#v, want %#v", got, want)
			}
		})
	}
}
