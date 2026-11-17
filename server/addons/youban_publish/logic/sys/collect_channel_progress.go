package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_publish/library/channelprogress"
	"hotgo/internal/library/microbatch"
)

// This is a synchronous metadata write, not a TTL throttle: the last message
// in a burst is persisted even if the channel receives no subsequent message.
func (s *sSysPublish) touchCollectChannelProgress(ctx context.Context, tenantID, channelID, messageID int64) error {
	if tenantID <= 0 || channelID <= 0 || messageID <= 0 {
		return gerror.New("采集频道进度归属无效")
	}
	s.tgQueueMu.Lock()
	if s.collectChannelProgressBatch == nil {
		s.collectChannelProgressBatch = microbatch.New(context.Background(), 50, 5*time.Millisecond, channelprogress.Write)
	}
	batch := s.collectChannelProgressBatch
	s.tgQueueMu.Unlock()
	_, err := batch.Do(ctx, channelprogress.Progress{TenantID: tenantID, ChannelID: channelID, MessageID: messageID})
	return err
}

func (s *sSysPublish) stopCollectChannelProgress() {
	s.tgQueueMu.Lock()
	batch := s.collectChannelProgressBatch
	s.collectChannelProgressBatch = nil
	s.tgQueueMu.Unlock()
	if batch != nil {
		batch.Close()
	}
}
