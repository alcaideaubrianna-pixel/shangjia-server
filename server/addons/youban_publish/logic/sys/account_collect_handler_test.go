package sys

import "testing"

func TestMatchAccountCollectSources(t *testing.T) {
	sources := []accountCollectSourceRuntime{
		{Id: 1, SourceChatId: "12345"},
		{Id: 2, SourceChatId: "-10012345"},
		{Id: 3, SourceChatId: "-678"},
		{Id: 4, SourceChatId: "999"},
	}
	matches := matchAccountCollectSources(sources, []string{"12345", "-10012345"})
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Id != 1 || matches[1].Id != 2 {
		t.Fatalf("unexpected matches: %+v", matches)
	}
}

func TestUniqueStrings(t *testing.T) {
	values := uniqueStrings([]string{" 1 ", "1", "", "2"})
	if len(values) != 2 || values[0] != "1" || values[1] != "2" {
		t.Fatalf("unexpected values: %+v", values)
	}
}
