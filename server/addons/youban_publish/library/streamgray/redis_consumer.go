package streamgray

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
)

const streamConsumerIdleWait = 100 * time.Millisecond

// StreamEntry 是 Redis Stream 消费适配层返回的最小消息结构。
type StreamEntry struct {
	ID      string
	Payload []byte
}

// RedisGroupClient 是 Consumer Group 所需的 Redis 最小接口。
type RedisGroupClient interface {
	ReadGroup(ctx context.Context, stream, group, consumer, startID string, count int, block time.Duration) ([]StreamEntry, error)
	Ack(ctx context.Context, stream, group, entryID string) error
}

// RedisGroupAdmin 是可选的 Consumer Group 初始化能力。
type RedisGroupAdmin interface {
	CreateGroup(context.Context, string, string) error
}

// RedisPendingClaimer 用于接管其他 consumer 遗留的超时 pending 消息。
type RedisPendingClaimer interface {
	ClaimPending(context.Context, string, string, string, time.Duration, int) ([]StreamEntry, error)
}

// RedisGroupConsumer 实现解码、处理、成功 ACK 的固定流程。
type RedisGroupConsumer struct {
	client        RedisGroupClient
	group         string
	consumer      string
	count         int
	block         time.Duration
	claimInterval time.Duration
}

// NewRedisGroupConsumer 创建 Redis Stream Consumer Group 消费器。
func NewRedisGroupConsumer(client RedisGroupClient, group, consumer string) (*RedisGroupConsumer, error) {
	if client == nil || group == "" || consumer == "" {
		return nil, gerror.New("Redis Stream consumer 参数无效")
	}
	return &RedisGroupConsumer{client: client, group: group, consumer: consumer, count: 20, block: 5 * time.Second, claimInterval: 30 * time.Second}, nil
}

// ConsumeOnce 消费一批消息；handler 成功后才 ACK，失败消息保留 pending。
func (c *RedisGroupConsumer) ConsumeOnce(ctx context.Context, stream, startID string, handler func(context.Context, ReadyEvent) error) (int, error) {
	if !validReadyStream(stream) || handler == nil {
		return 0, gerror.New("Redis Stream 消费参数无效")
	}
	entries, err := c.client.ReadGroup(ctx, stream, c.group, c.consumer, startID, c.count, c.block)
	if err != nil {
		return 0, gerror.Wrap(err, "读取 Redis Stream 消息失败")
	}
	return c.handleEntries(ctx, stream, entries, handler)
}

// handleEntries 统一处理新消息和接管消息，失败不确认，单频道竞争不影响同批其他频道。
func (c *RedisGroupConsumer) handleEntries(ctx context.Context, stream string, entries []StreamEntry, handler func(context.Context, ReadyEvent) error) (int, error) {
	processed := 0
	var retryErr error
	for _, entry := range entries {
		event, decodeErr := DecodeReadyEvent(entry.Payload)
		if decodeErr != nil {
			return processed, decodeErr
		}
		if err := handler(ctx, event); err != nil {
			if isTransientStreamHandlerError(err) {
				// 单频道竞争只保留当前消息，继续处理同批其他频道。
				retryErr = err
				continue
			}
			return processed, err
		}
		if err := c.client.Ack(ctx, stream, c.group, entry.ID); err != nil {
			return processed, gerror.Wrap(err, "确认 Redis Stream 消息失败")
		}
		processed++
	}
	return processed, retryErr
}

// Run 持续消费一个 Stream；启动时先读取 pending，再读取新消息。
// Redis 阻塞读取由 client 控制，空批次仅短暂让出调度，避免无消息时忙循环。
func (c *RedisGroupConsumer) Run(ctx context.Context, stream string, handler func(context.Context, ReadyEvent) error) error {
	if !validReadyStream(stream) || handler == nil {
		return gerror.New("Redis Stream 运行参数无效")
	}
	if err := c.ensureGroup(ctx, stream); err != nil {
		return err
	}
	startID := "0"
	pendingRetry := false
	nextClaim := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// 存活消费者也要回收其他实例崩溃后留下的 pending，不能只在启动时接管。
		// 每次只取一批，避免恢复大积压阻塞新频道。
		if claimer, ok := c.client.(RedisPendingClaimer); ok && !time.Now().Before(nextClaim) {
			nextClaim = time.Now().Add(c.claimInterval)
			entries, claimErr := claimer.ClaimPending(ctx, stream, c.group, c.consumer, 30*time.Second, c.count)
			if claimErr != nil {
				return gerror.Wrap(claimErr, "接管 Redis Stream pending 消息失败")
			}
			_, claimErr = c.handleEntries(ctx, stream, entries, handler)
			if claimErr != nil {
				if !isTransientStreamHandlerError(claimErr) {
					return claimErr
				}
				pendingRetry = true
			}
			if len(entries) == c.count {
				// 尚有恢复积压时加快下一批，但仍先让新消息执行。
				nextClaim = time.Now().Add(time.Second)
			}
		}
		processed, err := c.ConsumeOnce(ctx, stream, startID, handler)
		if err != nil {
			if isStreamGroupMissingError(err) {
				// Redis 重启或 Stream 被删除后 Consumer Group 会一并消失；
				// 仅在收到 NOGROUP 时重建，避免正常路径重复执行管理命令。
				if ensureErr := c.ensureGroup(ctx, stream); ensureErr != nil {
					return ensureErr
				}
				startID = ">"
				continue
			}
			if isTransientStreamHandlerError(err) {
				// 频道租约竞争、Redis 短暂抖动等错误不能让整个 Consumer
				// 退出；未 ACK 的消息会在下一轮从 pending 重新处理。
				// pending 与新消息交替处理，忙频道不能饿死新频道。
				pendingRetry = true
				retryingPending := startID == "0"
				if startID == "0" {
					startID = ">"
				} else {
					startID = "0"
				}
				if processed > 0 || retryingPending {
					// pending 仍忙时立即转向新消息，不给无关频道追加一秒延迟。
					// 新消息读取具有 BLOCK 超时，空批次还有 idleWait，避免空转。
					continue
				}
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
				continue
			}
			return err
		}
		// pending 读完后切换到新消息；后续始终使用 >，避免重复扫描历史消息。
		if startID == "0" {
			pendingRetry = false
			startID = ">"
		} else if pendingRetry {
			startID = "0"
		}
		if processed > 0 {
			continue
		}
		timer := time.NewTimer(streamConsumerIdleWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// ensureGroup 确保 Stream Consumer Group 存在，兼容 Redis 重启后的空 Stream。
func (c *RedisGroupConsumer) ensureGroup(ctx context.Context, stream string) error {
	admin, ok := c.client.(RedisGroupAdmin)
	if !ok {
		return nil
	}
	if err := admin.CreateGroup(ctx, stream, c.group); err != nil && !isGroupAlreadyExistsError(err) {
		return gerror.Wrap(err, "初始化 Redis Stream consumer group 失败")
	}
	return nil
}

// isGroupAlreadyExistsError 由具体 Redis 适配器将 BUSYGROUP 等错误映射为可忽略状态。
// 默认只识别稳定错误文本，避免吞掉连接或权限错误。
func isGroupAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "BUSYGROUP") || strings.Contains(strings.ToLower(err.Error()), "already exists")
}

// isStreamGroupMissingError 判断 Redis 是否报告 Consumer Group 已丢失。
func isStreamGroupMissingError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "NOGROUP")
}

// isTransientStreamHandlerError 判断消息处理是否应退避重试而保留 pending。
func isTransientStreamHandlerError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"lock failed", "稍后重试", "获取频道任务调度租约", "redis"} {
		if strings.Contains(message, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
