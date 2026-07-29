package sys

import (
	"errors"
	"testing"

	"github.com/gotd/td/tgerr"
)

func TestTGMediaTransferBestThreads(t *testing.T) {
	tests := []struct {
		name string
		size int64
		max  int
		want int
	}{
		{name: "small", size: 512 * 1024, max: 8, want: 1},
		{name: "medium", size: 4 * 1024 * 1024, max: 8, want: 2},
		{name: "large", size: 15 * 1024 * 1024, max: 2, want: 2},
		{name: "unknown", size: 0, max: 8, want: 8},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := tgMediaTransferBestThreads(test.size, test.max); got != test.want {
				t.Fatalf("tgMediaTransferBestThreads(%d, %d) = %d, want %d", test.size, test.max, got, test.want)
			}
		})
	}
}

func TestTGMediaTransferAuthInvalid(t *testing.T) {
	if !tgMediaTransferAuthInvalid(errors.New("rpc error: AUTH_BYTES_INVALID")) {
		t.Fatal("expected AUTH_BYTES_INVALID to be recognized")
	}
	if tgMediaTransferAuthInvalid(errors.New("context canceled")) {
		t.Fatal("context cancellation must not be treated as AUTH_BYTES_INVALID")
	}
}

func TestTGMediaTransferDCClosed(t *testing.T) {
	if !tgMediaTransferDCClosed(errors.New("get next chunk: DC is closed")) {
		t.Fatal("expected DC is closed to be recognized")
	}
	if tgMediaTransferDCClosed(errors.New("context canceled")) {
		t.Fatal("context cancellation must not be treated as DC is closed")
	}
}

func TestTGMediaTransferFileMigrate(t *testing.T) {
	dc, ok := tgMediaTransferFileMigrate(tgerr.New(303, "FILE_MIGRATE_4"))
	if !ok || dc != 4 {
		t.Fatalf("tgMediaTransferFileMigrate() = (%d, %t), want (4, true)", dc, ok)
	}
	if _, ok := tgMediaTransferFileMigrate(errors.New("FILE_MIGRATE")); ok {
		t.Fatal("malformed migration error must not be recognized")
	}
}

func TestTGMediaTransferRetiredConnectionClosesAfterRelease(t *testing.T) {
	closed := 0
	p := &tgMediaTransferAccountPool{}
	connection := &tgMediaTransferConnection{
		refs:  1,
		close: func() error { closed++; return nil },
	}
	connection.retired = true
	p.release(connection)
	if connection.refs != 0 || closed != 1 {
		t.Fatalf("released retired connection = refs:%d closed:%d, want refs:0 closed:1", connection.refs, closed)
	}
	p.release(connection)
	if closed != 1 {
		t.Fatalf("connection close called more than once: %d", closed)
	}
}
