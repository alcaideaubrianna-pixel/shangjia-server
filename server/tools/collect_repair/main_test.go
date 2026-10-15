package main

import (
	"reflect"
	"testing"
)

func TestParseEventIDs(t *testing.T) {
	got, err := parseEventIDs(" 4878297,4878302,4878297 ")
	if err != nil {
		t.Fatalf("parseEventIDs() error = %v", err)
	}
	want := []int64{4878297, 4878302}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEventIDs() = %v, want %v", got, want)
	}
}

func TestParseEventIDsRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "abc", "1,invalid"} {
		if _, err := parseEventIDs(value); err == nil {
			t.Fatalf("parseEventIDs(%q) unexpectedly succeeded", value)
		}
	}
}
