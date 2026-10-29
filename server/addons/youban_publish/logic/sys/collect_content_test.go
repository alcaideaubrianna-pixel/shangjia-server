package sys

import "testing"

func TestPreferCollectContentTextKeepsRicherDuplicate(t *testing.T) {
	full := "省份：山东\n城市：滨州\n年龄：18\n身高：153\n体重：78\n罩杯：D\n介绍费 6888"
	short := "省份：山东\n城市：滨州\n罩杯：D\n介绍费 6888"
	if preferCollectContentText(full, short) {
		t.Fatal("short duplicate must not replace richer caption")
	}
	if !preferCollectContentText(short, full) {
		t.Fatal("richer caption should replace short duplicate")
	}
}

func TestPreferCollectContentTextDoesNotOverwriteWithEmpty(t *testing.T) {
	if preferCollectContentText("已有正文", "") {
		t.Fatal("empty duplicate must not overwrite existing caption")
	}
}
