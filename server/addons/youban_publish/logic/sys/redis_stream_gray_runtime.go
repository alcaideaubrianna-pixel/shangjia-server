package sys

import (
	"context"
	"fmt"
	"os"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/library/streamgray"
)

const defaultRedisStreamGrayGroup = "youban_publish_push"

// runRedisStreamGrayConsumer 启动灰度 Stream 双流消费者。
// 该方法只在 Push Worker 且配置开关开启时调用，旧调度器不受影响。
func (s *sSysPublish) runRedisStreamGrayConsumer(ctx context.Context) {
	client, err := streamgray.NewRedisCommandGroupClientFromCache()
	if err != nil {
		g.Log().Warningf(ctx, "Redis Stream 灰度消费者未启动：%+v", err)
		return
	}
	group := g.Cfg().MustGet(ctx, "youbanPublish.redisStream.consumerGroup", defaultRedisStreamGrayGroup).String()
	if group == "" {
		group = defaultRedisStreamGrayGroup
	}
	consumer := fmt.Sprintf("%s-%d", os.Getenv("HOSTNAME"), os.Getpid())
	if consumer == "-" || consumer == "-0" {
		consumer = fmt.Sprintf("push-worker-%d", os.Getpid())
	}
	streamConsumer, err := streamgray.NewRedisGroupConsumer(client, group, consumer)
	if err != nil {
		g.Log().Warningf(ctx, "Redis Stream 灰度 Consumer 初始化失败：%+v", err)
		return
	}
	gate, dispatcher := s.newPublishStreamGrayBridge()
	handler := streamgray.NewGatedHandler(gate, dispatcher)
	g.Log().Infof(ctx, "Redis Stream 灰度消费者启动 group:%s consumer:%s", group, consumer)
	if err = streamgray.RunBothStreams(ctx, streamConsumer, handler); err != nil && ctx.Err() == nil {
		g.Log().Warningf(ctx, "Redis Stream 灰度消费者退出：%+v", err)
	}
}
