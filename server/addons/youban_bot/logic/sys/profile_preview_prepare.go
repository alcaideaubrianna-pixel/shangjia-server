package sys

import (
	"context"
	"os"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"golang.org/x/sync/errgroup"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

const (
	profilePreviewPrepareConcurrency = 3
	profilePreviewPrepareTimeout     = 8 * time.Second
)

// A process-local resource limit, not business state. Simultaneous previews
// must not turn fallback into an unbounded burst of downloads and open files.
var profilePreviewPrepareSlots = make(chan struct{}, 8)

// Prepare concurrently but keep the original album positions. Ownership of
// files passes to the caller only on success; failures close every opened file
// after all workers exit, including workers finishing during cancellation.
func prepareProfilePreviewAlbum(ctx context.Context, items []*publishsysin.MediaModel, captions []string,
	prepare func(context.Context, *publishsysin.MediaModel, string) (models.InputMedia, *os.File, error),
) ([]models.InputMedia, []*os.File, error) {
	if len(items) != len(captions) {
		return nil, nil, gerror.New("媒体组与文案数量不一致")
	}
	inputs := make([]models.InputMedia, len(items))
	files := make([]*os.File, len(items))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(profilePreviewPrepareConcurrency)
	for index, item := range items {
		group.Go(func() error {
			itemCtx, cancel := context.WithTimeout(groupCtx, profilePreviewPrepareTimeout)
			defer cancel()
			if err := itemCtx.Err(); err != nil {
				return err
			}
			select {
			case profilePreviewPrepareSlots <- struct{}{}:
				defer func() { <-profilePreviewPrepareSlots }()
			case <-itemCtx.Done():
				return itemCtx.Err()
			}
			if err := itemCtx.Err(); err != nil {
				return err
			}
			if item == nil {
				return gerror.New("媒体组包含空媒体")
			}
			input, file, err := prepare(itemCtx, item, captions[index])
			files[index] = file
			if err == nil {
				err = itemCtx.Err()
			}
			if err != nil {
				return gerror.Wrapf(err, "准备资料媒体失败，媒体编号：%d", item.Id)
			}
			if input == nil {
				return gerror.Newf("媒体组上传文件准备失败，媒体编号：%d", item.Id)
			}
			inputs[index] = input
			return nil
		})
	}
	err := group.Wait()
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		for _, file := range files {
			if file != nil {
				_ = file.Close()
			}
		}
		return nil, nil, err
	}
	opened := make([]*os.File, 0, len(files))
	for _, file := range files {
		if file != nil {
			opened = append(opened, file)
		}
	}
	return inputs, opened, nil
}
