package sys

import (
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
	"testing"
)

func originTestEvent(id int64, method string) gdb.Record {
	return gdb.Record{"id": g.NewVar(id), "tenant_id": g.NewVar(56), "account_id": g.NewVar(561), "source_id": g.NewVar(348), "source_type": g.NewVar(method), "source_chat_id": g.NewVar("4439615649"), "source_message_id": g.NewVar(1033), "source_grouped_id": g.NewVar("14333524971101069"), "source_name_snapshot": g.NewVar("historical title"), "source_username_snapshot": g.NewVar("lanrunrunff22"), "tg_account_id": g.NewVar(54), "received_at": g.NewVar("2026-10-11 11:50:21")}
}

func TestProfileOriginCoordinatesAndMethods(t *testing.T) {
	for _, method := range []string{"account", "bot", "follow", "unexpected"} {
		event := originTestEvent(4794056, method)
		if method == "bot" {
			event["source_chat_id"] = g.NewVar("-1004439615649")
			event["bot_id"] = g.NewVar(7)
		}
		if method == "follow" {
			event["source_chat_id"] = g.NewVar("account:22")
		}
		o := collectProfileOrigin(event, 654490)
		if o.GroupedId != "14333524971101069" || o.EventId != 4794056 || o.MessageId != 1033 {
			t.Fatalf("lost message/event distinction: %+v", o)
		}
		if method == "unexpected" {
			if o.CollectMethod != "unknown" || o.Completeness == "complete" {
				t.Fatal(o)
			}
			continue
		}
		if o.CollectMethod != method || o.Completeness != "complete" {
			t.Fatal(o)
		}
	}
	private := originTestEvent(1, "bot")
	private["source_chat_id"] = g.NewVar("1234567890")
	if got := collectProfileOrigin(private, 1).ChatId; got != "1234567890" {
		t.Fatalf("private user became a channel: %s", got)
	}
}

func TestProfileOriginKeyFallbackNeverInventsMethodOrOwner(t *testing.T) {
	key := "collect:v2:56:561:-1004439615649:group:14333524971101069"
	o := profileOriginFromKey(key, 56, 561, 654490)
	if o == nil || o.CollectMethod != "unknown" || o.Completeness != "partial" || o.MessageId != 0 {
		t.Fatal(o)
	}
	if profileOriginFromKey(key, 57, 561, 654490) != nil || profileOriginFromKey(key, 56, 562, 654490) != nil {
		t.Fatal("cross-owner key accepted")
	}
	if profileOriginFromKey("collect:v2:56:561:-1004439615649:other:1", 56, 561, 1) != nil {
		t.Fatal("invalid identity accepted")
	}
}

func TestProfileOriginFieldsRejectCrossOwnerAndMixedCoordinates(t *testing.T) {
	o := collectProfileOrigin(originTestEvent(1, "account"), 10)
	item := &sysin.ProfileModel{Id: 10, TenantId: 56, AccountId: 562, IsCollected: true}
	applyProfileOriginFields(item, []*sysin.ProfileOriginModel{o})
	if len(item.Origins) != 0 || item.CollectMethod != "unknown" {
		t.Fatalf("leaked owner: %+v", item)
	}
	item = &sysin.ProfileModel{Id: 10, TenantId: 56, AccountId: 561, CollectSourceChatId: "-1009999999999"}
	applyProfileOriginFields(item, []*sysin.ProfileOriginModel{o})
	if item.CollectSourceMessageId != 0 || item.CollectSourceUsername != "" {
		t.Fatal("mixed two source messages")
	}
}

func TestProfileOriginFieldsSelectMatchingCollector(t *testing.T) {
	first := collectProfileOrigin(originTestEvent(1, "account"), 10)
	event := originTestEvent(2, "bot")
	event["source_id"] = g.NewVar(349)
	event["source_chat_id"] = g.NewVar("-1004439615649")
	event["bot_id"] = g.NewVar(9)
	later := collectProfileOrigin(event, 10)
	item := &sysin.ProfileModel{Id: 10, TenantId: 56, AccountId: 561, CollectSourceId: 349, CollectSourceChatId: "-1004439615649", CollectSourceMessageId: 1033}
	applyProfileOriginFields(item, []*sysin.ProfileOriginModel{first, later})
	if item.CollectMethod != "bot" || len(item.Origins) != 2 {
		t.Fatalf("selected wrong source method: %+v", item)
	}
	item.CollectSourceMessageId = 5000
	item.CollectSourceUsername = ""
	applyProfileOriginFields(item, []*sysin.ProfileOriginModel{first, later})
	if item.CollectMethod != "unknown" || item.CollectSourceUsername != "" {
		t.Fatal("mixed observations")
	}
}
