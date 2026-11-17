package sys

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
)

// Opt-in disposable loopback database, isolated from other tests by subprocess.
// Exercise the real paginated lookup and pair transaction, not copied SQL.
func TestCollectMaterialPairAcrossPagesLocalPostgres(t *testing.T) {
	runCollectMaterialPairLocalPostgres(t, "-1003954466959")
}

func TestCollectMaterialPairShortChannelLocalPostgres(t *testing.T) {
	runCollectMaterialPairLocalPostgres(t, "-10042")
}

func TestCollectMaterialPairMixedChannelAliasesLocalPostgres(t *testing.T) {
	runCollectMaterialPairLocalPostgres(t, "-1003954466959", true)
}

func TestCollectMaterialOnlinePairStatesLocalPostgres(t *testing.T) {
	runCollectMaterialPairLocalPostgres(t, "-1003954466959", false, true)
}

func runCollectMaterialPairLocalPostgres(t *testing.T, chatID string, mixedAliases ...bool) {
	t.Helper()
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	redis := miniredis.RunT(t)
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + redis.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		statements := []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event(
 id bigint PRIMARY KEY,tenant_id bigint DEFAULT 1,account_id bigint DEFAULT 0,tg_account_id bigint DEFAULT 0,
 source_id bigint DEFAULT 1,source_chat_id text DEFAULT '-1003954466959',source_message_id bigint,source_grouped_id text DEFAULT '',
 raw_text text DEFAULT '',media_count int DEFAULT 1,media_json text DEFAULT '[]',status text DEFAULT 'pending',material_role text DEFAULT 'pending',material_group_status text DEFAULT '',
 material_parent_event_id bigint DEFAULT 0,error_message text DEFAULT '',processed_at timestamp,
 received_at timestamp DEFAULT LOCALTIMESTAMP,created_at timestamp DEFAULT LOCALTIMESTAMP,updated_at timestamp DEFAULT LOCALTIMESTAMP) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_event_log(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,event_id bigint,stage text,status text,message text,meta_text text,created_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_origin(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,raw_text,material_role) VALUES(5000,99,'这是一份尚未入库、错误关联了验证的旧展示资料，需要安全修复','display')`,
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,raw_text) VALUES(1,100,'这是一份展示资料，明确超过二十个字符，不能和下一份展示资料混合')`,
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,raw_text,media_count) SELECT 1000+n,101+n,'✅提交成功！',0 FROM generate_series(0,100) n`,
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,source_grouped_id,material_parent_event_id,material_role) VALUES(200,202,'verify-first',5000,'verify')`,
			`INSERT INTO hg_youban_publish_collect_event(id,tenant_id,source_message_id,raw_text) VALUES(3000,99,201,'这是另一个租户的展示资料，绝对不能成为当前验证资料的父级')`,
		}
		// The production media projection is intentionally explicit. Keep the
		// fixture complete as columns are added, without altering generated DAO.
		columns := make([]string, 0)
		for _, field := range strings.Split(collectEventMediaQueryFields(), ",") {
			typeName := "text"
			switch field {
			case "id":
				typeName = "bigint PRIMARY KEY"
			case "event_id":
				typeName = "bigint"
			case "created_at", "updated_at", "next_retry_at":
				typeName = "timestamp"
			}
			columns = append(columns, field+" "+typeName)
		}
		statements = append(statements,
			"CREATE TEMP TABLE hg_youban_publish_collect_event_media("+strings.Join(columns, ",")+") ON COMMIT DROP",
			`INSERT INTO hg_youban_publish_collect_event_media(id,event_id,media_type,source_file_id,cache_status) VALUES(1,1,'photo','photo','ready'),(2,200,'video','video','ready'),(3,3000,'photo','other','ready')`)
		for _, statement := range statements {
			if _, err := tx.Ctx(ctx).Exec(strings.ReplaceAll(statement, "-1003954466959", chatID)); err != nil {
				return err
			}
		}
		if len(mixedAliases) > 0 && mixedAliases[0] {
			// Keep display and intermediate rows in Bot API form, while the
			// verification and obsolete parent use the historical short ID.
			if _, err := tx.Ctx(ctx).Exec(`UPDATE hg_youban_publish_collect_event SET source_chat_id=? WHERE id IN (200,5000)`, canonicalCollectSourceChatID(chatID)); err != nil {
				return err
			}
		}
		// Time filtering must happen before predecessor ordering: these closer
		// message IDs are outside the verification window and cannot steal it.
		for _, sql := range []string{
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,raw_text,received_at) VALUES(6000,201,'这是窗口以外的旧展示资料，不能成为当前验证资料的父级',LOCALTIMESTAMP-INTERVAL '21 minutes')`,
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,raw_text,received_at) VALUES(7000,201,'这是未来接收的展示资料，不能成为当前验证资料的父级',LOCALTIMESTAMP+INTERVAL '1 minute')`,
			`UPDATE hg_youban_publish_collect_event SET received_at=NULL WHERE id=1`,
		} {
			if _, err := tx.Ctx(ctx).Exec(strings.ReplaceAll(sql, "-1003954466959", chatID)); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		verify, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=200`)
		if err != nil {
			return err
		}
		service := NewSysPublish()
		if _, err := service.findStoredCollectDisplayEvent(ctx, verify); !isCollectProcessRetryError(err) {
			t.Fatalf("first page must retain work for the next page: %v", err)
		}
		// Future ingress must not reset progress past the first 100 noise rows.
		if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,source_chat_id,raw_text) VALUES(9000,9000,?,'这是不影响前序配对的后续展示资料，文案超过二十字')`, chatID); err != nil {
			return err
		}
		payload := collectProcessQueuePayload{TenantId: 1, SourceId: 1, SourceChatId: chatID}
		if err := dirtyCollectTargetBatch(ctx, payload); err != nil {
			return err
		}
		display, err := service.findStoredCollectDisplayEvent(ctx, verify)
		if err != nil {
			return err
		}
		if display["id"].Int64() != 1 {
			t.Fatalf("wrong or missing scoped display: %v", display)
		}
		// The out-of-time fixtures exercise lookup filtering only. Canonical
		// adjacency is checked separately against every intervening message.
		if _, err := tx.Ctx(ctx).Exec(`DELETE FROM hg_youban_publish_collect_event WHERE id IN (6000,7000)`); err != nil {
			return err
		}
		// Unrelated future traffic changes the dirty generation but must not
		// prevent an already established finite interval from being bound.
		if _, err := tx.Ctx(ctx).Exec(`UPDATE hg_youban_publish_collect_event SET raw_text='这是被编辑成另一份展示的新文案，旧视频分类快照必须失效',updated_at=clock_timestamp() WHERE id=200`); err != nil {
			return err
		}
		if _, err := service.bindCollectMaterialPair(ctx, display, verify); !isCollectProcessRetryError(err) {
			t.Fatalf("edited verification reused a stale classification: %v", err)
		}
		if _, err := tx.Ctx(ctx).Exec(`UPDATE hg_youban_publish_collect_event SET raw_text='',updated_at=clock_timestamp() WHERE id=200`); err != nil {
			return err
		}
		for i := 0; i < 2; i++ {
			verify, err = tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=200`)
			if err != nil {
				return err
			}
			paired, err := service.bindCollectMaterialPair(ctx, display, verify)
			if err != nil {
				return err
			}
			if !paired.IsBound() {
				t.Fatal("cross-page pair was not bound idempotently")
			}
		}
		row, err := tx.Ctx(ctx).GetOne(`SELECT material_parent_event_id,material_role FROM hg_youban_publish_collect_event WHERE id=200`)
		if err == nil && (row["material_parent_event_id"].Int64() != 1 || row["material_role"].String() != "verify") {
			t.Fatalf("pair not persisted: %v", row)
		}
		if err == nil && len(mixedAliases) > 1 && mixedAliases[1] {
			return testCollectMaterialOnlinePairStates(ctx, t, tx, service)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Exact production IDs/states sampled through SLS + primary-key DBX reads on
// 2026-11-17. Captions/files are synthetic; no production contacts or media.
func testCollectMaterialOnlinePairStates(ctx context.Context, t *testing.T, tx gdb.TX, service *sSysPublish) error {
	t.Helper()
	for _, sample := range []struct {
		parent, verify, tenant, account, source, parentMessage, verifyMessage int64
		chat, status, role, verifyStatus, cacheStatus                         string
		want                                                                  publishconsts.CollectPairOutcome
		manual                                                                bool
	}{
		{17993730, 17993793, 56, 561, 1370, 8662, 8669, "-1004323635917", "processed", "display", "ignored", "canceled", publishconsts.CollectPairBound, false},
		{19078723, 19078824, 56, 561, 1370, 39150, 39153, "-1004456862377", "dispatched", "display", "pending", "pending", publishconsts.CollectPairBound, false},
		{19124819, 19125229, 56, 561, 1370, 39331, 39338, "-1004456862377", "dispatched", "display", "pending", "pending", publishconsts.CollectPairBound, false},
		{19138472, 19138760, 56, 561, 1370, 39409, 39416, "-1004456862377", "dispatched", "display", "pending", "pending", publishconsts.CollectPairBound, false},
		{19175181, 19175429, 56, 561, 1370, 39611, 39614, "-1004456862377", "dispatched", "display", "pending", "pending", publishconsts.CollectPairBound, false},
		{18930876, 18930968, 87, 628, 1147, 98548, 98553, "-1004325948289", "ignored", "display", "pending", "pending", publishconsts.CollectPairRejected, false},
		{18718928, 18718993, 87, 628, 1360, 490545, 490549, "-1001249875416", "ignored", "display", "pending", "pending", publishconsts.CollectPairRejected, false},
		{10242627, 10242734, 64, 569, 417, 20258, 20264, "3588792263", "processed", "verify", "pending", "pending", publishconsts.CollectPairUnresolved, true},
		{14082640, 14082655, 99, 647, 1481, 8060, 8064, "3736052878", "processed", "verify", "pending", "pending", publishconsts.CollectPairUnresolved, true},
		{17538565, 17538570, 33, 492, 1508, 1017, 1022, "4303045212", "ignored", "pending", "pending", "pending", publishconsts.CollectPairUnresolved, true},
		{18816184, 18816221, 62, 567, 1516, 1972, 1977, "-1003727917077", "ignored", "pending", "pending", "pending", publishconsts.CollectPairUnresolved, true},
	} {
		for _, table := range []string{"hg_youban_publish_collect_event", "hg_youban_publish_collect_event_media"} {
			if _, err := tx.Ctx(ctx).Exec("DELETE FROM " + table); err != nil {
				return err
			}
		}
		_, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event(id,tenant_id,account_id,source_id,source_chat_id,source_message_id,raw_text,status,material_role,material_group_status,error_message,received_at) VALUES(?,?,?,?,?,?,?,?,?,'complete','未命中关键词',?)`, sample.parent, sample.tenant, sample.account, sample.source, sample.chat, sample.parentMessage, strings.Repeat("资料", 20), sample.status, sample.role, gtime.New("2026-11-15 23:19:32"))
		if err != nil {
			return err
		}
		role := "pending"
		reason := ""
		if sample.verifyStatus == "ignored" {
			role = "verify"
			reason = collectMaterialVerifyUnmatchedMessage
		}
		_, err = tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event(id,tenant_id,account_id,source_id,source_chat_id,source_message_id,status,material_role,error_message,received_at,raw_text) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, sample.verify, sample.tenant, sample.account, sample.source, sample.chat, sample.verifyMessage, sample.verifyStatus, role, reason, gtime.New("2026-11-15 23:19:41"), strings.Repeat("\u200b", 66))
		if err != nil {
			return err
		}
		if _, err = tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event_media(id,event_id,media_type,source_file_id,cache_status) VALUES(1,?,'photo','photo','ready'),(2,?,'video','video',?)`, sample.parent, sample.verify, sample.cacheStatus); err != nil {
			return err
		}
		parent, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=?`, sample.parent)
		if err != nil {
			return err
		}
		verify, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=?`, sample.verify)
		if err != nil {
			return err
		}
		outcome, err := service.bindCollectMaterialPair(ctx, parent, verify)
		if sample.manual {
			if err != nil {
				return err
			}
			if outcome != publishconsts.CollectPairNeedsRepair {
				t.Fatalf("sample %d must quarantine uncertain historical parent: %v", sample.parent, outcome)
			}
			current, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=?`, sample.verify)
			if err != nil {
				return err
			}
			if current["material_parent_event_id"].Int64() != 0 || current["status"].String() != "failed" || !collectProcessEventFinished(current) {
				t.Fatalf("sample %d quarantine must preserve binding and stop hot retries: %v", sample.parent, current)
			}
			continue
		}
		if err != nil {
			return err
		}
		if outcome != sample.want {
			t.Fatalf("sample %d outcome=%v want=%v", sample.parent, outcome, sample.want)
		}
		current, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=?`, sample.verify)
		if err != nil {
			return err
		}
		if outcome.IsBound() {
			if current["material_parent_event_id"].Int64() != sample.parent {
				t.Fatalf("sample %d missing parent", sample.parent)
			}
			parent, err = tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=?`, sample.parent)
			if err != nil {
				return err
			}
			if !collectMaterialLateVerificationRepairCandidate(parent) {
				t.Fatalf("sample %d late parent would be ACKed", sample.parent)
			}
			outcome, err = service.bindCollectMaterialPair(ctx, parent, current)
			if err != nil {
				return err
			}
			if outcome != publishconsts.CollectPairAlreadyBound {
				t.Fatalf("sample %d retry lost idempotence", sample.parent)
			}
			media, err := tx.Ctx(ctx).GetOne(`SELECT cache_status FROM hg_youban_publish_collect_event_media WHERE id=2`)
			if err != nil {
				return err
			}
			if media["cache_status"].String() != "pending" {
				t.Fatalf("sample %d canceled verification was not restored", sample.parent)
			}
			if _, err = tx.Ctx(ctx).Exec(`UPDATE hg_youban_publish_collect_event_media SET cache_status='ready',file_url='https://example.invalid/local-test.mp4' WHERE id=2`); err != nil {
				return err
			}
			photoCount := map[int64]int{17993730: 6, 19078723: 3, 19124819: 7, 19138472: 7, 19175181: 3}[sample.parent]
			if photoCount == 0 {
				photoCount = 1
			}
			content := &collectContentResult{RawText: strings.Repeat("资料", 20), NormalizedText: "local fixture"}
			for i := 0; i < photoCount; i++ {
				content.Media = append(content.Media, collectMediaItem{Type: "photo", Purpose: collectMaterialRoleDisplay, FileId: strings.Repeat("p", i+1)})
			}
			merged, err := service.mergeCollectMaterialContent(ctx, parent, content)
			if err != nil {
				return err
			}
			if !collectContentHasVerification(merged) || len(merged.Media) != photoCount+1 {
				t.Fatalf("sample %d material did not retain %d display media plus one verification", sample.parent, photoCount)
			}
			// One display still accepts only one logical verification group.
			if _, err = tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event(id,tenant_id,account_id,source_id,source_chat_id,source_message_id,source_grouped_id,received_at) VALUES(?,?,?,?,?,?,?,?)`, sample.verify+1, sample.tenant, sample.account, sample.source, sample.chat, sample.verifyMessage+1, "second-group", gtime.New("2026-11-15 23:19:42")); err != nil {
				return err
			}
			second, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=?`, sample.verify+1)
			if err != nil {
				return err
			}
			outcome, err = service.bindCollectMaterialPair(ctx, parent, second)
			if err != nil {
				return err
			}
			if outcome != publishconsts.CollectPairRejected {
				t.Fatalf("sample %d accepted a second verification group", sample.parent)
			}
		} else if current["status"].String() != "ignored" {
			t.Fatalf("sample %d rejection not persisted", sample.parent)
		}
	}
	return nil
}
