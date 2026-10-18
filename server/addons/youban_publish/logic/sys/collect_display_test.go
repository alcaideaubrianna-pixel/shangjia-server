package sys

import "testing"

func TestCollectSourceTitleNeedsEnrichment(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		channelID string
		want      bool
	}{
		{name: "empty title", channelID: "-1003715905169", want: true},
		{name: "same full id", title: "-1003715905169", channelID: "-1003715905169", want: true},
		{name: "same normalized id", title: "3715905169", channelID: "-1003715905169", want: true},
		{name: "custom title", title: "百花阁 BOT 采集", channelID: "-1003715905169", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectSourceTitleNeedsEnrichment(test.title, test.channelID); got != test.want {
				t.Fatalf("collectSourceTitleNeedsEnrichment(%q, %q) = %v, want %v", test.title, test.channelID, got, test.want)
			}
		})
	}
}

func TestCollectSourceSharedChannelNormalizesTelegramID(t *testing.T) {
	cache := make(map[string]collectSourceChannelCacheValue)
	mergeCollectSourceSharedChannel(cache, "-1003715905169", "阿水一群", "uushsh886")
	value, ok := collectSourceSharedChannel("3715905169", cache)
	if !ok || value.Title != "阿水一群" || value.Username != "uushsh886" {
		t.Fatalf("unexpected channel cache value: %#v, exists=%v", value, ok)
	}
}
