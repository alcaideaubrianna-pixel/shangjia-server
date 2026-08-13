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
