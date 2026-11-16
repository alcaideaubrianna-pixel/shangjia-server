package sys

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestProfileTaskStatusRequiresSelector(t *testing.T) {
	for _, in := range []*sysin.ProfileViewInp{nil, {}, {Id: -1, Uuid: " ", ProfileNo: " "}} {
		_, err := (&sSysPublish{}).profileTaskStatusBySelector(context.Background(), in)
		if err == nil || !strings.Contains(err.Error(), "资料ID、UUID或编号不能为空") {
			t.Fatalf("expected selector validation error for %+v, got %v", in, err)
		}
	}
}

func TestProfileTaskStatusResponseJSONRemainsCompatible(t *testing.T) {
	body, err := json.Marshal(&sysin.ProfileTaskStatusModel{
		Id:         813124,
		Uuid:       "b9wtfi01000dmvefwxi5eoa1rnthobdn",
		Status:     2,
		TaskStatus: "unpublishing",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err = json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{
		"id":         float64(813124),
		"uuid":       "b9wtfi01000dmvefwxi5eoa1rnthobdn",
		"status":     float64(2),
		"taskStatus": "unpublishing",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response JSON changed: %s", body)
	}
}
