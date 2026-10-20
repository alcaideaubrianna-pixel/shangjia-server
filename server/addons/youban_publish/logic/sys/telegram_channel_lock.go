package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
	hglock "hotgo/internal/library/hgrds/lock"
	"hotgo/utility/ownedlease"
)

const telegramChannelLeaseTTL = telegramSendBudget + 30*time.Second

func (s *sSysPublish) withTelegramChannelLock(ctx context.Context, chatId string, fn func() error) error {
	key := telegramChannelLockKey(chatId)
	lock := s.telegramChannelLock(chatId)
	lock.Lock()
	defer lock.Unlock()
	return s.withTelegramChannelDBLock(ctx, key, fn)
}

func (s *sSysPublish) tryTelegramChannelLease(ctx context.Context, chatId string) (*hglock.Lock, bool, error) {
	key := telegramChannelLockKey(chatId)
	// Telegram/network calls can ignore cancellation while a worker is being
	// recovered. A fixed lease prevents such a caller from blocking the channel
	// forever through watchdog renewal.
	lease := hglock.NewConfig(telegramChannelLeaseTTL, time.Second).WithoutWatchDog().Mutex(key)
	if err := lease.TryLock(ctx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return nil, false, nil
		}
		return nil, false, gerror.Wrap(err, "获取TG频道发送令牌失败")
	}
	return lease, true, nil
}

type telegramChannelLease interface {
	Unlock(context.Context) error
}

type telegramPublishLeaseOwner struct {
	Kind     string `json:"kind"`
	JobId    int64  `json:"jobId"`
	TenantId int64  `json:"tenantId"`
}

// Only a durable, complete send is reclaimable. In-flight, uncertain, failed,
// cleanup and legacy leases retain their original expiry protection.
func telegramCompletedLeaseRecoverable(owner telegramPublishLeaseOwner, job telegramJobRecord, chatId string) bool {
	return owner.Kind == "telegram_publish_v1" && owner.JobId > 0 && owner.TenantId > 0 &&
		job.Id == owner.JobId && job.TenantId == owner.TenantId &&
		normalizeTelegramChannelChatID(job.TargetChatId) == normalizeTelegramChannelChatID(chatId) &&
		!isDownTelegramOperationNo(job.OperationNo) && !isMessagePushOperationNo(job.OperationNo) &&
		job.Status == telegramJobStatusSent &&
		(job.SendPhase == telegramSendPhaseVerifyConfirmed || job.SendPhase == telegramSendPhaseCompletedNoVerify)
}

func (s *sSysPublish) tryTelegramPublishChannelLease(ctx context.Context, job telegramJobRecord) (telegramChannelLease, bool, error) {
	return tryTelegramPublishChannelLease(ctx, job, s.telegramJobById)
}

func tryTelegramPublishChannelLease(ctx context.Context, job telegramJobRecord, lookup func(context.Context, int64) (telegramJobRecord, error), clients ...*gredis.Redis) (telegramChannelLease, bool, error) {
	key := telegramChannelLockKey(job.TargetChatId)
	ownerJSON, _ := json.Marshal(telegramPublishLeaseOwner{Kind: "telegram_publish_v1", JobId: job.Id, TenantId: job.TenantId})
	lease := ownedlease.New(key, string(ownerJSON), telegramChannelLeaseTTL, clients...)
	ok, err := lease.TryAcquire(ctx)
	if err != nil || ok {
		return lease, ok, err
	}
	snapshot, err := ownedlease.Inspect(ctx, key, clients...)
	if err != nil {
		return nil, false, gerror.Wrap(err, "读取频道发送租约归属失败")
	}
	var owner telegramPublishLeaseOwner
	if json.Unmarshal([]byte(snapshot.Owner), &owner) != nil || owner.Kind != "telegram_publish_v1" || owner.JobId <= 0 || owner.TenantId != job.TenantId {
		return nil, false, nil
	}
	previous, err := lookup(ctx, owner.JobId)
	if err != nil {
		return nil, false, err
	}
	beginTelegramPublishStage(ctx, "channel.send_lease.owner").End(nil, g.Map{
		"ownerJobId": previous.Id, "ownerStatus": previous.Status, "ownerSendPhase": previous.SendPhase,
		"sameJob": previous.Id == job.Id, "ownerJobAgeMs": telegramPublishTimestampAge(previous.CreatedAt),
	})
	if !telegramCompletedLeaseRecoverable(owner, previous, job.TargetChatId) {
		return nil, false, nil
	}
	released, err := ownedlease.ReleaseObserved(ctx, key, snapshot, clients...)
	if err != nil || !released {
		return nil, false, err
	}
	g.Log().Infof(ctx, "自动释放已完成TG任务遗留发送租约 previousJobId:%d nextJobId:%d chat:%s", previous.Id, job.Id, job.TargetChatId)
	ok, err = lease.TryAcquire(ctx)
	return lease, ok, err
}

func (s *sSysPublish) releaseTelegramChannelLease(ctx context.Context, lease telegramChannelLease) {
	if lease == nil {
		return
	}
	// A canceled send context must not leave the channel locked for the full
	// task timeout. Unlock still compares the lease token before deleting it.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := lease.Unlock(ctx); err != nil && !gerror.Is(err, hglock.ErrNotExist) {
		g.Log().Warningf(ctx, "释放TG频道发送令牌失败：%+v", err)
	}
}

func (s *sSysPublish) telegramChannelLock(chatId string) *publishRuntimeMutex {
	key := telegramChannelLockKey(chatId)
	s.telegramChannelMu.Lock()
	defer s.telegramChannelMu.Unlock()
	if s.telegramChannelLocks == nil {
		s.telegramChannelLocks = make(map[string]*publishRuntimeMutex)
	}
	lock := s.telegramChannelLocks[key]
	if lock == nil {
		lock = &publishRuntimeMutex{}
		s.telegramChannelLocks[key] = lock
	}
	return lock
}

func telegramChannelLockKey(chatId string) string {
	key := strings.TrimSpace(normalizeTelegramChannelChatID(chatId))
	if key == "" {
		key = "default"
	}
	return "youban_publish:tg_channel:" + key
}

func (s *sSysPublish) withTelegramChannelDBLock(ctx context.Context, key string, fn func() error) error {
	dbType := strings.ToLower(g.DB().GetConfig().Type)
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		switch dbType {
		case consts.DBPgsql:
			if err := configureTelegramChannelLockTimeout(tx); err != nil {
				return err
			}
			if _, err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", key); err != nil {
				return gerror.Wrap(err, "获取TG频道发送锁失败")
			}
			return fn()
		default:
			value, err := tx.GetValue("SELECT GET_LOCK(?, 60)", key)
			if err != nil {
				return gerror.Wrap(err, "获取TG频道发送锁失败")
			}
			if value.Int() != 1 {
				return gerror.New("获取TG频道发送锁超时")
			}
			defer func() {
				_, _ = tx.Exec("SELECT RELEASE_LOCK(?)", key)
			}()
			return fn()
		}
	})
}

// This transaction holds only the cross-instance channel lock while the callback
// performs Telegram I/O. A short database idle timeout must not release the lock
// before the existing send budget expires. LOCAL keeps ordinary transactions and
// pooled connections on their configured timeout; zero (disabled) is preserved.
func configureTelegramChannelLockTimeout(tx gdb.TX) error {
	minimum := telegramChannelLeaseTTL.Milliseconds()
	_, err := tx.Exec(`SELECT set_config('idle_in_transaction_session_timeout',
		CASE WHEN setting::bigint > 0 AND setting::bigint < ?
		THEN ? ELSE setting || 'ms' END, true)
		FROM pg_settings WHERE name='idle_in_transaction_session_timeout'`, minimum, fmt.Sprintf("%dms", minimum))
	return gerror.Wrap(err, "配置TG频道持锁事务超时失败")
}
