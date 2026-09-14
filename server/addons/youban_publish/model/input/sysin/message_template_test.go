package sysin

import (
	"context"
	"testing"
)

func TestMessageTemplateSaveFilterDefaultsOriginalMedia(t *testing.T) {
	in := &MessageTemplateSaveInp{
		Name: "template",
		Media: []*MessageTemplateMediaInp{{
			MediaType:   "image",
			FileUrl:     " /attachment/current.jpg ",
			StoragePath: " attachment/current.jpg ",
		}},
	}

	if err := in.Filter(context.Background()); err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	media := in.Media[0]
	if media.OriginalFileUrl != "/attachment/current.jpg" || media.OriginalStoragePath != "attachment/current.jpg" {
		t.Fatalf("original media was not defaulted from current media: %+v", media)
	}
	if media.EditStatus != "raw" {
		t.Fatalf("EditStatus = %q, want raw", media.EditStatus)
	}
}

func TestMessageTemplateSaveFilterEditStatus(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{name: "raw", status: "raw"},
		{name: "edited", status: "edited"},
		{name: "invalid", status: "processing", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := &MessageTemplateSaveInp{
				Name: "template",
				Media: []*MessageTemplateMediaInp{{
					MediaType:  "image",
					FileUrl:    "/attachment/current.jpg",
					EditStatus: test.status,
				}},
			}

			err := in.Filter(context.Background())
			if (err != nil) != test.wantErr {
				t.Fatalf("Filter() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && in.Media[0].EditStatus != test.status {
				t.Fatalf("EditStatus = %q, want %q", in.Media[0].EditStatus, test.status)
			}
		})
	}
}
