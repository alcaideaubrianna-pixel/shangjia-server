package sys

import (
	"testing"
	"time"
)

// Synthetic source metadata from incidents, with no personal text or assets.
// M19327 has two display groups then a verification: the video belongs to
// source display 12072 even though fingerprint dedupe reuses the first profile.
func TestCollectIdentitySourceAdjacencyReplay(t *testing.T) {
	now := time.Now()
	display := func(id int64) collectMaterialMessageView {
		return collectMaterialMessageView{EventID: id, RawText: "昵称：sample\n省份：上海\n城市：上海\n年龄：21", Media: []collectMediaItem{{Type: "photo"}}, MessageAt: now}
	}
	verify := func(id int64, group string) collectMaterialMessageView {
		return collectMaterialMessageView{EventID: id, GroupedID: group, Media: []collectMediaItem{{Type: "video"}}, MessageAt: now.Add(time.Minute)}
	}
	for _, tc := range []struct {
		name            string
		events          []collectMaterialMessageView
		display, verify int
		count           int
	}{
		{"M19327 repeated display keeps actual parent", []collectMaterialMessageView{display(20303151), display(20303280), verify(20303409, "")}, 1, 2, 1},
		{"T40073 late verification retains source parent", []collectMaterialMessageView{display(19666762), verify(19818424, "")}, 0, 1, 1},
		{"second independent video cannot attach", []collectMaterialMessageView{display(1), verify(2, ""), verify(3, "")}, 0, 1, 1},
		{"one album remains one logical verification", []collectMaterialMessageView{display(1), verify(2, "album"), verify(3, "album")}, 0, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pairs := pairCollectMaterialMessages(tc.events)
			if len(pairs) != tc.count {
				t.Fatalf("pairs=%+v", pairs)
			}
			if pairs[0].DisplayIndex != tc.display || pairs[0].VerifyIndex != tc.verify {
				t.Fatalf("wrong source parent: %+v", pairs)
			}
		})
	}
}
