package sys

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/lib/pq"
)

func TestCollectTerminalFailureClassification(t *testing.T) {
	overflow := &pq.Error{Code: "22001", Message: "value too long for type character varying(255)"}
	for _, tc := range []struct {
		name          string
		err           error
		terminal      bool
		configuration bool
	}{
		{"title", gerror.New("采集资料标题为空"), true, false},
		{"varchar", gerror.Wrap(overflow, "创建采集资料失败"), true, false},
		{"timeout", context.DeadlineExceeded, false, false},
		{"db_lock", &pq.Error{Code: "55P03"}, false, false},
		{"db_connection", &pq.Error{Code: "08006"}, false, false},
		{"db_disk", &pq.Error{Code: "53100"}, false, false},
		{"mixed_overflow_timeout", errors.Join(overflow, context.DeadlineExceeded), false, false},
		{"channels_missing", gerror.Wrap(gerror.New("采集规则未配置目标频道"), "分支失败"), false, true},
		{"channels_disabled", gerror.New("存在无权操作或不可用的推送频道"), false, true},
		{"mixed_config_timeout", errors.Join(gerror.New("采集规则未配置目标频道"), context.DeadlineExceeded), false, false},
		{"sql_text_not_sqlstate", errors.New("query text mentions value too long"), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := collectTerminalProcessingFailure(tc.err); got != tc.terminal {
				t.Fatalf("terminal=%t want=%t", got, tc.terminal)
			}
			if got := collectConfigurationFailure(tc.err); got != tc.configuration {
				t.Fatalf("configuration=%t want=%t", got, tc.configuration)
			}
		})
	}
}

func TestCollectConfigurationWaitAndRepair(t *testing.T) {
	rules := []gdb.Record{{"id": g.NewVar(1)}, {"id": g.NewVar(2)}}
	for _, message := range []string{"采集规则未配置目标频道", "存在无权操作或不可用的推送频道"} {
		called := 0
		result := executeCollectRuleBranches(context.Background(), rules, func(_ context.Context, rule gdb.Record) (bool, string, error) {
			called++
			if rule["id"].Int() == 1 {
				return false, "", gerror.New(message)
			}
			return true, "", nil
		})
		var retry *collectProcessRetryError
		if called != 2 || !result.Matched || result.terminalProcessingFailure() || !errors.As(result.Err, &retry) || retry.delay != 5*time.Minute || !strings.Contains(result.Err.Error(), message) {
			t.Fatalf("config lost sibling or retry reason: %+v", result)
		}
		repaired := executeCollectRuleBranches(context.Background(), rules, func(context.Context, gdb.Record) (bool, string, error) { return true, "", nil })
		if repaired.Err != nil || !repaired.Matched {
			t.Fatalf("repaired config remained blocked: %+v", repaired)
		}
	}
}

func TestCollectExtractedTitleRespectsStorageLimit(t *testing.T) {
	for _, title := range []string{strings.Repeat("a", 300), strings.Repeat("资料🌸", 100), strings.Repeat("中", 255), "短标题"} {
		text := "昵称：" + title
		original := text
		got := collectTitle(text)
		want := len([]rune(title))
		if want > 255 {
			want = 255
		}
		if len([]rune(got)) != want || !utf8.ValidString(got) || text != original {
			t.Fatalf("title limit invalid: runes=%d want=%d", len([]rune(got)), want)
		}
	}
	if got := collectTitle("  \n "); got != "" {
		t.Fatalf("empty content invented title: %q", got)
	}
}

func TestCollectTerminalInputDoesNotHideWaitingSibling(t *testing.T) {
	for _, sibling := range []error{nil, context.DeadlineExceeded, newCollectProcessRetryError(time.Second, "等待媒体")} {
		errs := []error{gerror.New("采集资料标题为空"), sibling}
		r := executeCollectRuleBranches(context.Background(), []gdb.Record{{"id": g.NewVar(1)}, {"id": g.NewVar(2)}}, func(_ context.Context, row gdb.Record) (bool, string, error) {
			err := errs[row["id"].Int()-1]
			return err == nil, "", err
		})
		if r.terminalProcessingFailure() != (sibling == nil) {
			t.Fatalf("incorrect terminal decision for sibling %v: %+v", sibling, r)
		}
	}
}

func TestCollectTerminalSiblingKeepsConfigurationBackoff(t *testing.T) {
	r := executeCollectRuleBranches(context.Background(), []gdb.Record{{"id": g.NewVar(1)}, {"id": g.NewVar(2)}}, func(_ context.Context, row gdb.Record) (bool, string, error) {
		if row["id"].Int() == 1 {
			return false, "", gerror.New("采集资料标题为空")
		}
		return false, "", gerror.New("采集规则未配置目标频道")
	})
	var retry *collectProcessRetryError
	if r.terminalProcessingFailure() || !errors.As(r.Err, &retry) || retry.delay != collectConfigurationRetryDelay || !strings.Contains(r.Err.Error(), "采集规则未配置目标频道") {
		t.Fatalf("terminal sibling bypassed config cooldown: %+v", r)
	}
}
