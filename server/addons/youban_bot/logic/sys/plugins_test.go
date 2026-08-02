package sys

import "testing"

func TestStartFeatureWelcomeImageSchema(t *testing.T) {
	for _, item := range (startFeature{}).ConfigSchema() {
		if item.Field != "welcomeImage" {
			continue
		}
		if item.Component != "image_upload_general" {
			t.Fatalf("unexpected welcome image component: %s", item.Component)
		}
		return
	}
	t.Fatal("welcomeImage schema not found")
}
