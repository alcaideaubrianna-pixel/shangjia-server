package sys

import "testing"

func TestAntiScanSegmentPresentationURLUsesConfiguredCDN(t *testing.T) {
	path := "hotgo/file/anti-scan/segment/35e1614ab4602796.png"
	want := mediaContentCDNBaseURL() + "/" + path
	got := antiScanSegmentPresentationURL("https://img.yuebanby.com/" + path)
	if got != want {
		t.Fatalf("unexpected segment URL: got %q want %q", got, want)
	}
}

func TestAntiScanSegmentPresentationURLKeepsExternalURL(t *testing.T) {
	want := "https://example.com/provider/result.png"
	if got := antiScanSegmentPresentationURL(want); got != want {
		t.Fatalf("external URL was rewritten: got %q want %q", got, want)
	}
}
