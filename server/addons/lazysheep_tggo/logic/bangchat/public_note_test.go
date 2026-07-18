package bangchat

import "testing"

func TestPublicNoteCodeFromURLSupportsNoteListCode(t *testing.T) {
	code, ok := publicNoteCodeFromURL("https://note.bangchat.icu/note/list?code=eff402b0-eb60-4122-b8a1-d6229d9e6924")
	if !ok {
		t.Fatal("expected note.bangchat.icu note list url to be supported")
	}
	if code != "eff402b0-eb60-4122-b8a1-d6229d9e6924" {
		t.Fatalf("unexpected code: %s", code)
	}
}
