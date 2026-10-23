package sysin

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestMessageTemplateButtonConfigAllowsMoreThanEightButtons(t *testing.T) {
	rows := make([][]MessageTemplateButton, 3)
	for rowIndex := range rows {
		for buttonIndex := 0; buttonIndex < 4; buttonIndex++ {
			index := rowIndex*4 + buttonIndex + 1
			rows[rowIndex] = append(rows[rowIndex], MessageTemplateButton{
				Text: fmt.Sprintf("button-%d", index),
				URL:  fmt.Sprintf("https://example.com/%d", index),
			})
		}
	}
	encoded, err := json.Marshal(MessageTemplateButtonConfig{Mode: "inline", Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	in := &MessageTemplateSaveInp{ButtonConfig: string(encoded)}
	if err = in.NormalizeButtonConfig(); err != nil {
		t.Fatalf("NormalizeButtonConfig() rejected 12 buttons: %v", err)
	}
	var normalized MessageTemplateButtonConfig
	if err = json.Unmarshal([]byte(in.ButtonConfig), &normalized); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range normalized.Rows {
		count += len(row)
	}
	if count != 12 {
		t.Fatalf("normalized button count = %d, want 12", count)
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
