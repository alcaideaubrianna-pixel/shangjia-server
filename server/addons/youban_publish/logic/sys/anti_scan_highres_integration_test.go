package sys

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"testing"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// Run manually with production rendering and a real FAPIHub key:
// YOUBAN_FAPIHUB_TEST_IMAGE=/abs/input.jpg \
// YOUBAN_FAPIHUB_TEST_OUTPUT=/abs/output.png \
// YOUBAN_FAPIHUB_API_KEY=... \
// go test ./addons/youban_publish/logic/sys -run TestFapiHubHighResolutionProductionRender -count=1
func TestFapiHubHighResolutionProductionRender(t *testing.T) {
	imagePath := os.Getenv("YOUBAN_FAPIHUB_TEST_IMAGE")
	apiKey := os.Getenv("YOUBAN_FAPIHUB_API_KEY")
	segmentPath := os.Getenv("YOUBAN_FAPIHUB_SEGMENT_IMAGE")
	segmentOutput := os.Getenv("YOUBAN_FAPIHUB_SEGMENT_OUTPUT")
	outputPath := os.Getenv("YOUBAN_FAPIHUB_TEST_OUTPUT")
	if imagePath == "" || outputPath == "" || (apiKey == "" && segmentPath == "") {
		t.Skip("set image/output and either YOUBAN_FAPIHUB_API_KEY or YOUBAN_FAPIHUB_SEGMENT_IMAGE")
	}
	source, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	var segment []byte
	if segmentPath != "" {
		segment, err = os.ReadFile(segmentPath)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		client := newFapiHubClient(apiKey, "https://fapihub.com/v2/rembg/", "falcon")
		segment, err = client.removeBackground(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		if segmentOutput != "" {
			if err := os.WriteFile(segmentOutput, segment, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	detect := &antiScanDetectResult{Provider: "fapihub-matting", SegmentRaw: encodeFapiHubSegmentPortrait(segment)}
	in := &sysin.AntiScanPreviewInp{AntiScanConfig: model.AntiScanConfig{BackgroundReplaceEnabled: 1, BackgroundTexturePreset: "rabbit", StickerOpacity: 30}}
	output, _, err := renderAntiScanPreviewLossless(context.Background(), source, in, detect)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, output, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("production render output=%s size=%dx%d bytes=%d", outputPath, config.Width, config.Height, len(output))
}
