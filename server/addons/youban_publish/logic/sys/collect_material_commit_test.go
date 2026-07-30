package sys

import "testing"

func TestValidateCollectMaterialMediaRequiresEveryMediaAsset(t *testing.T) {
	tests := []struct {
		name    string
		content *collectContentResult
		wantErr bool
	}{
		{
			name: "all assets prepared",
			content: &collectContentResult{
				MediaCount: 2,
				MediaJSON:  `[{"type":"photo","storagePath":"cache/a.jpg"},{"type":"video","fileUrl":"https://cdn.test/a.mp4"}]`,
			},
		},
		{
			name: "missing asset path",
			content: &collectContentResult{
				MediaCount: 2,
				MediaJSON:  `[{"type":"photo","storagePath":"cache/a.jpg"},{"type":"video"}]`,
			},
			wantErr: true,
		},
		{
			name: "media count mismatch",
			content: &collectContentResult{
				MediaCount: 2,
				MediaJSON:  `[{"type":"photo","storagePath":"cache/a.jpg"}]`,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCollectMaterialMedia(test.content)
			if (err != nil) != test.wantErr {
				t.Fatalf("validate error=%v, want error=%t", err, test.wantErr)
			}
		})
	}
}

func TestValidateCollectMaterialMediaKeepsVerifyAssetInGate(t *testing.T) {
	content := &collectContentResult{
		MediaCount: 2,
		MediaJSON:  `[{"type":"photo","storagePath":"cache/a.jpg","purpose":"display"},{"type":"video","purpose":"verify"}]`,
	}

	if err := validateCollectMaterialMedia(content); err == nil {
		t.Fatal("expected verify media without a prepared path to block commit")
	}
}

func TestCollectPreparedMediaCounts(t *testing.T) {
	imageCount, videoCount, hasVerificationVideo := collectPreparedMediaCounts([]collectPreparedMedia{
		{MediaType: "image", Purpose: collectMaterialRoleDisplay},
		{MediaType: "video", Purpose: collectMaterialRoleDisplay},
		{MediaType: "video", Purpose: collectMaterialRoleVerify},
	})

	if imageCount != 1 || videoCount != 2 || hasVerificationVideo != 1 {
		t.Fatalf("counts=(%d, %d, %d), want (1, 2, 1)", imageCount, videoCount, hasVerificationVideo)
	}
}
