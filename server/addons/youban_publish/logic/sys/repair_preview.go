package sys

// RepairPreviewText exposes the production collection text normalizer for
// offline audit tools. It performs no database writes and does not publish.
func RepairPreviewText(text, original, suffix string) string {
	return applyCollectIntroFeeSuffix(text, original, suffix)
}
