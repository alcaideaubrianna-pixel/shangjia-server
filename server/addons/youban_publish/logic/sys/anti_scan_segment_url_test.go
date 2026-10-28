package sys

import "testing"

func TestAntiScanSegmentPresentationURLUsesConfiguredCDN(t *testing.T) {
	path := "hotgo/file/anti-scan/segment/35e1614ab4602796.png"
	want := mediaContentCDNBaseURL() + "/" + path
	got := antiScanSegmentPresentationURL("https://sto.xiao-feiji.cc/" + path)
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

func TestAntiScanSegmentPresentationURLRewritesAttachment(t *testing.T) {
	path := "hotgo/attachment/a4/a4c0f143b4ae35a63c4746b9528ad35a.png"
	got := antiScanSegmentPresentationURL("http://xiaohuiji-tg-os.oss-ap-southeast-1.aliyuncs.com/" + path)
	want := mediaContentCDNBaseURL() + "/" + path
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAntiScanSegmentPresentationURLRewritesFile(t *testing.T) {
	path := "hotgo/file/08/08fd70e555da1fc8c3c88284b35f6140.png"
	got := antiScanSegmentPresentationURL("http://xiaohuiji-tg-os.oss-ap-southeast-1.aliyuncs.com/" + path)
	want := mediaContentCDNBaseURL() + "/" + path
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
