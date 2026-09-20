package sys

import (
	"testing"

	"hotgo/internal/consts"
	"hotgo/internal/library/storager"
	"hotgo/internal/model"
)

func TestNormalizeMediaFileURLUsesCosPublicURLForRelativeObject(t *testing.T) {
	previous := storager.GetConfig()
	storager.SetConfig(&model.UploadConfig{
		Drive:        consts.UploadDriveCos,
		CosBucketURL: "https://bucket.cos.ap-hongkong.myqcloud.com",
		CosPublicURL: "https://img.example.com",
	})
	t.Cleanup(func() { storager.SetConfig(previous) })

	path := "hotgov1/attachment/2026-08-13/example.png"
	if actual := normalizeMediaFileURL(path, path); actual != "https://img.example.com/"+path {
		t.Fatalf("unexpected COS public URL: %q", actual)
	}
}

func TestNormalizeMediaPresentationURLUsesConfiguredCDN(t *testing.T) {
	path := "hotgo/file/2026-09-14/example.mp4"
	want := mediaContentCDNBaseURL() + "/" + path
	if actual := normalizeMediaPresentationURL("https://sto.xiao-feiji.cc/"+path, path); actual != want {
		t.Fatalf("unexpected presentation URL: got %q want %q", actual, want)
	}
}

func TestNormalizeMediaPresentationURLRewritesManagedLegacyURLWithoutStoragePath(t *testing.T) {
	previous := storager.GetConfig()
	storager.SetConfig(&model.UploadConfig{
		Drive:        consts.UploadDriveCos,
		CosBucketURL: "https://bucket.cos.ap-hongkong.myqcloud.com",
		CosPublicURL: "https://cos.xiao-feiji.cc",
	})
	t.Cleanup(func() { storager.SetConfig(previous) })

	path := "hotgo/file/2026-09-14/example.jpg"
	want := mediaContentCDNBaseURL() + "/" + path + "?preview=1"
	actual := normalizeMediaPresentationURL("https://sto.xiao-feiji.cc/"+path+"?preview=1", "")
	if actual != want {
		t.Fatalf("unexpected legacy presentation URL: got %q want %q", actual, want)
	}
}

func TestNormalizeMediaPresentationURLKeepsExternalSourceURL(t *testing.T) {
	raw := "https://cdn.telegram.org/source/example.jpg"
	if actual := normalizeMediaPresentationURL(raw, ""); actual != raw {
		t.Fatalf("external URL was rewritten: got %q want %q", actual, raw)
	}
}
