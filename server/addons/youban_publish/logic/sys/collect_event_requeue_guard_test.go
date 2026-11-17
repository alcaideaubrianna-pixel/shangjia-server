package sys

import (
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func collectRequeueGuardFixture() (collectProcessQueuePayload, gdb.Record, gdb.Record, gdb.Record) {
	display := gdb.Record{
		"id": g.NewVar(19182174), "tenant_id": g.NewVar(78), "account_id": g.NewVar(0),
		"source_id": g.NewVar(351), "source_type": g.NewVar(sysin.CollectSourceTypeBot),
		"source_chat_id": g.NewVar("-1003763752459"), "bot_id": g.NewVar(64), "tg_account_id": g.NewVar(0),
		"material_role": g.NewVar(collectMaterialRoleDisplay), "material_parent_event_id": g.NewVar(0),
		"status": g.NewVar(sysin.CollectEventStatusIgnored), "error_message": g.NewVar(collectRequeueLegacyChannelReason),
	}
	verify := gdb.Record{}
	for field, value := range display {
		verify[field] = value
	}
	verify["id"] = g.NewVar(19182205)
	verify["material_role"] = g.NewVar(collectMaterialRoleVerify)
	verify["material_parent_event_id"] = display["id"]
	verify["source_chat_id"] = g.NewVar("3763752459")
	source := gdb.Record{
		"id": g.NewVar(351), "tenant_id": g.NewVar(78), "account_id": g.NewVar(615),
		"source_type": g.NewVar(sysin.CollectSourceTypeBot), "bot_id": g.NewVar(64), "tg_account_id": g.NewVar(0),
	}
	payload := collectProcessQueuePayload{
		EventId: 19182174, TenantId: 78, AccountId: 615, SourceId: 351,
		SourceChatId: "3763752459", RuleId: 338,
	}
	return payload, display, verify, source
}

func TestCollectRequeueRawZeroAndPairedAliasPreserved(t *testing.T) {
	payload, display, verify, source := collectRequeueGuardFixture()
	for _, accountID := range []int64{0, 615} {
		payload.AccountId = accountID
		if err := validateCollectRequeueDisplay(payload, display, source); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateCollectRequeuePair(display, verify); err != nil {
		t.Fatal(err)
	}
	if display["account_id"].Int64() != 0 || verify["account_id"].Int64() != 0 {
		t.Fatal("preflight must not migrate persisted account ownership")
	}
	if payload.RuleId != 338 || verify["material_parent_event_id"].Int64() != 19182174 {
		t.Fatal("preflight must preserve rule and existing pair identity")
	}
}

func TestCollectRequeueRejectsForeignScope(t *testing.T) {
	for _, field := range []string{"tenant", "source", "account", "chat"} {
		t.Run(field, func(t *testing.T) {
			payload, display, _, source := collectRequeueGuardFixture()
			switch field {
			case "tenant":
				payload.TenantId = 79
			case "source":
				payload.SourceId = 352
			case "account":
				payload.AccountId = 616
			case "chat":
				payload.SourceChatId = "-1003856373273"
			}
			if err := validateCollectRequeueDisplay(payload, display, source); err == nil {
				t.Fatal("foreign request was accepted")
			}
		})
	}
}

func TestCollectRequeueDoesNotReopenOtherIgnoredReasons(t *testing.T) {
	for _, reason := range []string{
		"采集频道已禁用", "机器人已被移出采集频道", "删除关键字忽略:test",
		"未命中规则或被屏蔽", "父展示资料已忽略", collectMaterialVerifyUnmatchedMessage,
		collectRequeueLegacyChannelReason + " ", "",
	} {
		t.Run(reason, func(t *testing.T) {
			payload, display, verify, source := collectRequeueGuardFixture()
			display["error_message"] = g.NewVar(reason)
			if err := validateCollectRequeueDisplay(payload, display, source); err == nil {
				t.Fatal("unrelated ignored parent was accepted")
			}
			_, display, _, _ = collectRequeueGuardFixture()
			verify["error_message"] = g.NewVar(reason)
			if err := validateCollectRequeuePair(display, verify); err == nil {
				t.Fatal("unrelated ignored child was accepted")
			}
		})
	}
}

func TestCollectRequeuePairRequiresOriginalScopeAndParent(t *testing.T) {
	for _, field := range []string{"tenant_id", "account_id", "source_id", "bot_id", "tg_account_id", "material_parent_event_id", "source_chat_id"} {
		t.Run(field, func(t *testing.T) {
			_, display, verify, _ := collectRequeueGuardFixture()
			verify[field] = g.NewVar(999)
			if err := validateCollectRequeuePair(display, verify); err == nil {
				t.Fatal("foreign paired verification was accepted")
			}
		})
	}
	_, display, verify, _ := collectRequeueGuardFixture()
	display["status"] = g.NewVar(sysin.CollectEventStatusMediaReady)
	if err := validateCollectRequeuePair(display, verify); err == nil {
		t.Fatal("ignored child without matching ignored parent evidence was accepted")
	}
}

func TestCollectRequeueNormalRepairStatesRemainAccepted(t *testing.T) {
	for _, status := range []string{
		sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect,
		sysin.CollectEventStatusPrechecked, sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady, sysin.CollectEventStatusProcessed,
		sysin.CollectEventStatusDispatched, sysin.CollectEventStatusFailed,
	} {
		payload, display, verify, source := collectRequeueGuardFixture()
		display["status"], verify["status"] = g.NewVar(status), g.NewVar(status)
		if err := validateCollectRequeueDisplay(payload, display, source); err != nil {
			t.Fatalf("normal %s repair rejected: %v", status, err)
		}
		if err := validateCollectRequeuePair(display, verify); err != nil {
			t.Fatalf("normal %s paired repair rejected: %v", status, err)
		}
	}
}

func TestCollectRequeueCanceledMediaRequiresExactCauseAndRawOwner(t *testing.T) {
	_, display, _, _ := collectRequeueGuardFixture()
	media := gdb.Record{
		"tenant_id": display["tenant_id"], "account_id": display["account_id"],
		"source_id": display["source_id"], "source_chat_id": g.NewVar("3763752459"),
		"cache_status": g.NewVar(collectMediaCacheCanceled), "error_message": g.NewVar(collectRequeueLegacyChannelReason),
	}
	if err := validateCollectRequeueMedia(display, media); err != nil {
		t.Fatal(err)
	}
	media["error_message"] = g.NewVar("人工取消")
	if err := validateCollectRequeueMedia(display, media); err == nil {
		t.Fatal("unrelated media cancellation accepted")
	}
	media["error_message"] = g.NewVar(collectRequeueLegacyChannelReason)
	media["account_id"] = g.NewVar(615)
	if err := validateCollectRequeueMedia(display, media); err == nil {
		t.Fatal("resolved owner must not replace raw media owner")
	}
}

func TestCollectRequeueInheritedReasonRequiresSameLegacyParent(t *testing.T) {
	payload, display, verify, source := collectRequeueGuardFixture()
	inherited := "父展示资料已忽略：" + collectRequeueLegacyChannelReason
	verify["error_message"] = g.NewVar(inherited)
	if err := validateCollectRequeuePair(display, verify); err != nil {
		t.Fatal(err)
	}
	if !collectRequeueRestoreIgnored(display, verify) {
		t.Fatal("the explicitly paired inherited authorization error must recover")
	}
	media := gdb.Record{
		"event_id": verify["id"], "tenant_id": display["tenant_id"],
		"account_id": display["account_id"], "source_id": display["source_id"],
		"source_chat_id": verify["source_chat_id"], "cache_status": g.NewVar(collectMediaCacheCanceled),
		"error_message": g.NewVar(inherited),
	}
	if err := validateCollectRequeueMedia(display, media); err != nil {
		t.Fatal(err)
	}
	// This prefix is evidence only for a child of the exact ignored parent;
	// it never expands the allowed reasons of a parent display.
	display["error_message"] = g.NewVar(inherited)
	if err := validateCollectRequeueDisplay(payload, display, source); err == nil {
		t.Fatal("inherited reason must not authorize a parent display")
	}
	if err := validateCollectRequeuePair(display, verify); err == nil {
		t.Fatal("child without original parent incident must not recover")
	}
	if err := validateCollectRequeueMedia(display, media); err == nil {
		t.Fatal("inherited canceled media without parent evidence must not recover")
	}
	_, display, _, _ = collectRequeueGuardFixture()
	verify["error_message"] = g.NewVar(inherited + "；人工取消")
	if err := validateCollectRequeuePair(display, verify); err == nil {
		t.Fatal("suffix or fuzzy inherited reason must not recover")
	}
}

func TestCollectRequeueGeneratedDONullOverridesSurviveORMConversion(t *testing.T) {
	event := collectRequeueEventUpdate{
		YoubanPublishCollectEvent: pdo.YoubanPublishCollectEvent{
			Status: sysin.CollectEventStatusGroupCollect, ErrorMessage: "",
			// The generated inner ProcessedAt intentionally remains nil.
		},
		ProcessedAt: gdb.Raw("NULL"),
	}
	media := collectRequeueMediaUpdate{
		YoubanPublishCollectEventMedia: pdo.YoubanPublishCollectEventMedia{
			CacheStatus: collectMediaCachePending, ErrorMessage: "", DownloadErrorType: "",
			// The generated inner NextRetryAt intentionally remains nil.
		},
		NextRetryAt: gdb.Raw("NULL"),
	}
	for _, tc := range []struct {
		name      string
		value     any
		nullField string
		innerName string
		stateKey  string
		state     string
	}{
		{"event_value", event, "ProcessedAt", "processed_at", "Status", sysin.CollectEventStatusGroupCollect},
		{"event_pointer", &event, "ProcessedAt", "processed_at", "Status", sysin.CollectEventStatusGroupCollect},
		{"media_value", media, "NextRetryAt", "next_retry_at", "CacheStatus", collectMediaCachePending},
		{"media_pointer", &media, "NextRetryAt", "next_retry_at", "CacheStatus", collectMediaCachePending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// This exported gdb conversion uses the same ORM tag priority and
			// OmitEmpty=true as Data's struct-to-map conversion. No DB needed.
			fields := gdb.MapOrStructToMapDeep(tc.value, true)
			raw, ok := fields[tc.nullField].(gdb.Raw)
			if !ok || raw != gdb.Raw("NULL") {
				t.Fatalf("%s lost explicit SQL NULL: %#v", tc.nullField, fields)
			}
			if _, exists := fields[tc.innerName]; exists {
				t.Fatalf("inner generated timestamp must not shadow SQL NULL: %#v", fields)
			}
			if fields[tc.stateKey] != tc.state {
				t.Fatalf("generated DO state was lost: %#v", fields)
			}
			if value, exists := fields["ErrorMessage"]; !exists || value != "" {
				t.Fatalf("explicit empty error must survive DO conversion: %#v", fields)
			}
			if tc.stateKey == "CacheStatus" {
				if value, exists := fields["DownloadErrorType"]; !exists || value != "" {
					t.Fatalf("explicit empty download error must survive: %#v", fields)
				}
			}
		})
	}
}
