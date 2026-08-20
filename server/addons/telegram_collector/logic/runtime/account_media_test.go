package runtime

import (
	"errors"
	"testing"
	"time"

	"github.com/gotd/td/tgerr"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountMediaDownloadTimeout(t *testing.T) {
	tests := []struct {
		size int64
		want time.Duration
	}{
		{size: 2 << 20, want: time.Minute},
		{size: 20 << 20, want: 2 * time.Minute},
		{size: 80 << 20, want: 3 * time.Minute},
	}
	for _, test := range tests {
		if got := accountMediaDownloadTimeout(test.size); got != test.want {
			t.Fatalf("accountMediaDownloadTimeout(%d)=%s want=%s", test.size, got, test.want)
		}
	}
}

func TestAccountMediaReferenceExpired(t *testing.T) {
	if !accountMediaReferenceExpired(errors.New("FILE_REFERENCE_EXPIRED")) || !accountMediaReferenceExpired(errors.New("file_reference_invalid")) {
		t.Fatal("expected file reference errors to be refreshable")
	}
	if accountMediaReferenceExpired(errors.New("FILE_MIGRATE_4")) {
		t.Fatal("FILE_MIGRATE must not be treated as reference expiration")
	}
}

func TestAccountMediaLocation(t *testing.T) {
	location, ok := accountMediaLocation(sysin.CollectorMediaItem{
		SourceKind: sysin.MediaKindPhoto, SourceMediaID: 1, SourceAccessHash: 2, SourceFileReference: []byte("ref"),
	})
	if !ok || location == nil {
		t.Fatal("expected photo location")
	}
}

func TestAccountMediaTransferBestThreads(t *testing.T) {
	tests := []struct {
		size      int64
		max, want int
	}{
		{size: 512 * 1024, max: 8, want: 1},
		{size: 4 * 1024 * 1024, max: 8, want: 2},
		{size: 15 * 1024 * 1024, max: 2, want: 2},
		{size: 0, max: 8, want: 1},
	}
	for _, test := range tests {
		if got := accountMediaBestThreads(test.size, test.max); got != test.want {
			t.Fatalf("accountMediaBestThreads(%d,%d)=%d want=%d", test.size, test.max, got, test.want)
		}
	}
}

func TestNormalizeAccountMediaThreads(t *testing.T) {
	tests := []struct {
		value int
		want  int
	}{
		{value: accountMediaDefaultThreads, want: 6},
		{value: 0, want: 1},
		{value: accountMediaMaxThreads + 1, want: accountMediaMaxThreads},
	}
	for _, test := range tests {
		if got := normalizeAccountMediaThreads(test.value); got != test.want {
			t.Fatalf("normalizeAccountMediaThreads(%d)=%d want=%d", test.value, got, test.want)
		}
	}
}

func TestAccountMediaConcurrencyMatchesRuntimeCapacity(t *testing.T) {
	if accountTaskWorkerCount != 8 {
		t.Fatalf("account task worker count = %d", accountTaskWorkerCount)
	}
	if accountMediaPoolSize < int64(accountTaskWorkerCount) {
		t.Fatalf("media pool size %d is smaller than task worker count %d", accountMediaPoolSize, accountTaskWorkerCount)
	}
}

func TestAccountMediaTransferErrors(t *testing.T) {
	if !accountMediaConnectionInvalid(errors.New("AUTH_BYTES_INVALID")) || !accountMediaConnectionInvalid(errors.New("DC is closed")) {
		t.Fatal("expected connection error classification")
	}
	dc, ok := accountMediaFileMigrate(tgerr.New(303, "FILE_MIGRATE_4"))
	if !ok || dc != 4 {
		t.Fatalf("accountMediaFileMigrate()=(%d,%t)", dc, ok)
	}
}

func TestAccountMediaTransferRetiredConnectionClosesAfterRelease(t *testing.T) {
	closed := 0
	pool := &accountMediaTransferPool{}
	connection := &accountMediaTransferConnection{refs: 1, close: func() error { closed++; return nil }, retired: true}
	pool.release(connection)
	if connection.refs != 0 || closed != 1 {
		t.Fatalf("unexpected release refs:%d closed:%d", connection.refs, closed)
	}
}
