package sys

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

func TestProfilePreviewPrepareClosesFilesOnFailure(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		name := "error"
		if cancelled {
			name = "cancelled"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			ready := make(chan struct{})
			var opened *os.File
			failure := errors.New("download failed")
			inputs, files, err := prepareProfilePreviewAlbum(ctx,
				[]*publishsysin.MediaModel{{Id: 1}, {Id: 2}}, []string{"first", "second"},
				func(itemCtx context.Context, item *publishsysin.MediaModel, caption string) (models.InputMedia, *os.File, error) {
					if item.Id == 1 {
						var openErr error
						opened, openErr = os.CreateTemp(t.TempDir(), "preview")
						close(ready)
						if openErr != nil {
							return nil, nil, openErr
						}
						<-itemCtx.Done()
						// Even a download finishing during cancellation may return an open file.
						return &models.InputMediaPhoto{Media: caption}, opened, nil
					}
					select {
					case <-ready:
					case <-itemCtx.Done():
						return nil, nil, itemCtx.Err()
					}
					if cancelled {
						cancel()
						return nil, nil, ctx.Err()
					}
					return nil, nil, failure
				})
			if err == nil || inputs != nil || files != nil {
				t.Fatalf("partial album escaped: inputs=%v files=%v err=%v", inputs, files, err)
			}
			if !cancelled && !errors.Is(err, failure) {
				t.Fatalf("original failure lost: %v", err)
			}
			if opened == nil {
				t.Fatal("file was not opened")
			}
			if _, err := opened.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("file leaked: %v", err)
			}
		})
	}
}

func TestProfilePreviewPreparePreservesOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	second := make(chan struct{})
	inputs, files, err := prepareProfilePreviewAlbum(ctx,
		[]*publishsysin.MediaModel{{Id: 1}, {Id: 2}}, []string{"first", "second"},
		func(itemCtx context.Context, item *publishsysin.MediaModel, caption string) (models.InputMedia, *os.File, error) {
			if item.Id == 1 {
				select {
				case <-second:
				case <-itemCtx.Done():
					return nil, nil, itemCtx.Err()
				}
			} else {
				close(second)
			}
			return &models.InputMediaPhoto{Media: caption}, nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || len(files) != 0 || inputs[0].(*models.InputMediaPhoto).Media != "first" || inputs[1].(*models.InputMediaPhoto).Media != "second" {
		t.Fatalf("album reordered: %v", inputs)
	}
}
