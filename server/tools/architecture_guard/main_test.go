package main

import (
	"os"
	"path/filepath"
	"testing"
)

func sample(t *testing.T, source string) []entry {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(p, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := inspect("addons/youban_publish/logic/sys/sample.go", p)
	if err != nil {
		t.Fatal(err)
	}
	return items
}
func TestGuardRejectsBypasses(t *testing.T) {
	for _, tt := range []struct{ name, body, rule string }{
		{"aliased Redis", `import gf "github.com/gogf/gf/v2/frame/g";func bad(){gf.Redis()}`, "redis-primitive"},
		{"Lua", `func bad(){client.GroupScript().Eval(ctx, script,1,keys,args)}`, "redis-primitive"},
		{"state map", `func bad(){data:=map[string]any{"status":"sent"};_ = data}`, "state-map-write"},
		{"state index", `func bad(){data["dispatch_status"]="done"}`, "state-index-write"},
		{"state selector", `func bad(){job.SendPhase="display_confirmed"}`, "state-selector-write"},
		{"cast", `func bad(){state:=JobStatus(raw);_ = state}`, "state-cast"},
		{"qualified cast", `func bad(){state:=sysin.PublishTaskStatus(raw);_ = state}`, "state-cast"},
		{"raw command", `func bad(){client.Do(ctx,"SET",key,value)}`, "redis-primitive"},
		{"raw SQL", `func bad(){db.Exec(ctx,"UPDATE hg_youban_publish_tg_job SET status='sent' WHERE id=?",id)}`, "state-sql-write"},
		{"bare Job", `type telegramJobRecord struct{Status string}`, "untyped-job-field"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items := sample(t, "package sys;"+tt.body)
			matched := false
			for _, v := range items {
				if v.Rule == tt.rule {
					matched = true
				}
			}
			if !matched || len(compare(items, nil)) == 0 {
				t.Fatalf("bypass not rejected: %+v", items)
			}
		})
	}
}

func TestGuardRejectsCollectorPublishCouplingAndGlobalScans(t *testing.T) {
	tests := []struct {
		name string
		body string
		rule string
	}{
		{"publish import", `import _ "hotgo/addons/youban_publish/logic/sys"`, "collector-publish-coupling"},
		{"telegram job state query", `func bad(){db.Raw("SELECT status FROM hg_youban_publish_tg_job WHERE id=?")}`, "collector-tg-state-sql"},
		{"unbounded collection query", `func bad(){db.Raw("SELECT id FROM hg_youban_publish_collect_event")}`, "collector-global-sql-scan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "sample.go")
			if err := os.WriteFile(p, []byte("package collector;"+tt.body), 0600); err != nil {
				t.Fatal(err)
			}
			items, err := inspect("addons/telegram_collector/logic/sys/sample.go", p)
			if err != nil {
				t.Fatal(err)
			}
			matched := false
			for _, item := range items {
				if item.Rule == tt.rule {
					matched = true
				}
			}
			if !matched || len(compare(items, nil)) == 0 {
				t.Fatalf("collector bypass not rejected: %+v", items)
			}
		})
	}
}

func TestGuardAllowsBoundedCollectorQuery(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(p, []byte(`package collector;func good(){db.Raw("SELECT id FROM hg_youban_publish_collect_event WHERE source_id=? ORDER BY id LIMIT 100")}`), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := inspect("addons/telegram_collector/logic/sys/sample.go", p)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Rule == "collector-global-sql-scan" {
			t.Fatalf("bounded collector query was flagged: %+v", items)
		}
	}
}
func TestGuardBaselineIsExact(t *testing.T) {
	old := sample(t, `package sys;func old(){data["status"]="sent"}`)
	if len(compare(old, old)) != 0 {
		t.Fatal("exact existing expression rejected")
	}
	changed := sample(t, `package sys;func old(){data["status"]="pending"}`)
	if len(compare(changed, old)) == 0 {
		t.Fatal("changed expression escaped baseline")
	}
	moved := sample(t, `package sys;func newEntry(){data["status"]="sent"}`)
	if len(compare(moved, old)) == 0 {
		t.Fatal("function-wide exception escaped")
	}
	duplicate := append(append([]entry{}, old...), old...)
	duplicate[0].Count = 2
	if len(compare(duplicate[:1], old)) == 0 {
		t.Fatal("duplicate new call escaped baseline")
	}
	broad := append([]entry{}, old...)
	broad[0].Function = "*"
	if len(compare(old, broad)) == 0 {
		t.Fatal("broad exception accepted")
	}
}
func TestGuardAllowsDomainEntry(t *testing.T) {
	items := sample(t, `package sys;func good(){service.transitionTelegramJob(ctx,transition);queue.Mark(ctx,scope,Changed)}`)
	if len(items) != 0 {
		t.Fatalf("domain entry was blocked: %+v", items)
	}
}

func TestGuardExactPointerReceiver(t *testing.T) {
	items := sample(t, `package sys;func(s *sSysPublish) legacy(){job.Status="sent"}`)
	if len(items) == 0 || len(compare(items, items)) != 0 {
		t.Fatalf("exact pointer receiver rejected: %+v", items)
	}
	for _, fn := range []string{"*", "*sSysPublish.*", "sSysPublish.?", "[abc].run"} {
		bad := append([]entry(nil), items...)
		bad[0].Function = fn
		if len(compare(items, bad)) == 0 {
			t.Fatalf("glob accepted: %s", fn)
		}
	}
}
func TestGuardIgnoresUnrelatedStatus(t *testing.T) {
	items := sample(t, `package sys;func good(){response.Status=401;filter.Status=1;table.Status="status";data:=map[string]any{"status":1};data["status"]=2}`)
	if len(items) != 0 {
		t.Fatalf("unrelated HTTP/filter/metadata flagged: %+v", items)
	}
}
func TestGuardProtectsNamedJobType(t *testing.T) {
	items := sample(t, `package sys;func bad(other *telegramJobRecord){other.Status=next;data:=map[string]any{"status":next};data["status"]=next}`)
	if len(items) != 3 {
		t.Fatalf("typed job state bypass missed: %+v", items)
	}
}
func TestGuardScansNewSubpackage(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "addons/youban_publish/new/package")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte(`package novel;func bad(){job.Status="sent"}`), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || len(compare(items, nil)) == 0 {
		t.Fatal("new package bypass missed")
	}
}
