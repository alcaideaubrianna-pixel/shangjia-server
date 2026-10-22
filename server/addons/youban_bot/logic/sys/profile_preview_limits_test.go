package sys

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

func TestProfilePreviewPrepareDeadlineCancelsWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	var active atomic.Int32
	_, _, err := prepareProfilePreviewAlbum(ctx, []*publishsysin.MediaModel{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}}, make([]string, 4),
		func(ctx context.Context, _ *publishsysin.MediaModel, _ string) (models.InputMedia, *os.File, error) {
			active.Add(1)
			defer active.Add(-1)
			<-ctx.Done()
			return nil, nil, ctx.Err()
		})
	if !errors.Is(err, context.DeadlineExceeded) || active.Load() != 0 {
		t.Fatalf("workers=%d err=%v", active.Load(), err)
	}
}

func TestProfilePreviewPrepareItemDeadline(t *testing.T) {
	_, _, err := prepareProfilePreviewAlbum(context.Background(), []*publishsysin.MediaModel{{Id: 1}}, []string{""},
		func(ctx context.Context, _ *publishsysin.MediaModel, _ string) (models.InputMedia, *os.File, error) {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > profilePreviewPrepareTimeout {
				t.Error("missing item deadline")
			}
			<-ctx.Done()
			return nil, nil, ctx.Err()
		})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}

func TestProfilePreviewPrepareGlobalLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var active, peak atomic.Int32
	ready := make(chan struct{})
	var once sync.Once
	prepare := func(ctx context.Context, _ *publishsysin.MediaModel, _ string) (models.InputMedia, *os.File, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		if n == int32(cap(profilePreviewPrepareSlots)) {
			once.Do(func() { close(ready) })
		}
		select {
		case <-ready:
			return &models.InputMediaPhoto{Media: "ready"}, nil, nil
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
	var workers sync.WaitGroup
	errorsCh := make(chan error, 4)
	for i := 0; i < 4; i++ {
		workers.Go(func() {
			_, _, err := prepareProfilePreviewAlbum(ctx, []*publishsysin.MediaModel{{Id: 1}, {Id: 2}, {Id: 3}}, make([]string, 3), prepare)
			errorsCh <- err
		})
	}
	workers.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if peak.Load() != int32(cap(profilePreviewPrepareSlots)) || active.Load() != 0 || len(profilePreviewPrepareSlots) != 0 {
		t.Fatalf("peak=%d active=%d slots=%d", peak.Load(), active.Load(), len(profilePreviewPrepareSlots))
	}
}

func TestProfilePreviewPrepareInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name      string
		items     []*publishsysin.MediaModel
		captions  []string
		nilResult bool
	}{
		{name: "mismatched", items: []*publishsysin.MediaModel{{Id: 1}}},
		{name: "nil_media", items: []*publishsysin.MediaModel{nil}, captions: []string{""}},
		{name: "nil_result", items: []*publishsysin.MediaModel{{Id: 1}}, captions: []string{""}, nilResult: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := prepareProfilePreviewAlbum(context.Background(), test.items, test.captions,
				func(context.Context, *publishsysin.MediaModel, string) (models.InputMedia, *os.File, error) {
					if !test.nilResult {
						t.Error("invalid input reached downloader")
					}
					return nil, nil, nil
				})
			if err == nil {
				t.Fatal("accepted invalid album")
			}
		})
	}
}
