package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/library/cache"
)

const (
	accountCollectCircuitInitialDelay = 5 * time.Second
	accountCollectCircuitMaxDelay     = 5 * time.Minute
	accountCollectCircuitCacheTTL     = 24 * time.Hour
	accountCollectCircuitCachePrefix  = "youban_publish:collect:account_circuit:"
)

type accountCollectCircuit struct {
	failures     int
	blockedUntil time.Time
	permanent    bool
	status       string
	lastMessage  string
	updatedAt    time.Time
	recoveredAt  time.Time
	recoveries   int
}

type accountCollectCircuitSnapshot struct {
	Failures     int       `json:"failures"`
	BlockedUntil time.Time `json:"blockedUntil"`
	Permanent    bool      `json:"permanent"`
	Status       string    `json:"status"`
	LastMessage  string    `json:"lastMessage"`
	UpdatedAt    time.Time `json:"updatedAt"`
	RecoveredAt  time.Time `json:"recoveredAt"`
	Recoveries   int       `json:"recoveries"`
}

func (state accountCollectCircuit) MarshalJSON() ([]byte, error) {
	return json.Marshal(accountCollectCircuitSnapshot{
		Failures:     state.failures,
		BlockedUntil: state.blockedUntil,
		Permanent:    state.permanent,
		Status:       state.status,
		LastMessage:  state.lastMessage,
		UpdatedAt:    state.updatedAt,
		RecoveredAt:  state.recoveredAt,
		Recoveries:   state.recoveries,
	})
}

func (state *accountCollectCircuit) UnmarshalJSON(data []byte) error {
	var snapshot accountCollectCircuitSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	state.failures = snapshot.Failures
	state.blockedUntil = snapshot.BlockedUntil
	state.permanent = snapshot.Permanent
	state.status = snapshot.Status
	state.lastMessage = snapshot.LastMessage
	state.updatedAt = snapshot.UpdatedAt
	state.recoveredAt = snapshot.RecoveredAt
	state.recoveries = snapshot.Recoveries
	return nil
}

func accountCollectCircuitCacheKey(tgAccountId int64) string {
	return fmt.Sprintf("%s%d", accountCollectCircuitCachePrefix, tgAccountId)
}

func (s *sSysPublish) restoreAccountCollectCircuit(ctx context.Context, tgAccountId int64) {
	if s == nil || tgAccountId <= 0 {
		return
	}
	s.accountCircuitMu.Lock()
	_, exists := s.accountCircuits[tgAccountId]
	s.accountCircuitMu.Unlock()
	if exists {
		return
	}
	state := accountCollectCircuit{}
	func() {
		defer func() { _ = recover() }()
		value, err := cache.Instance().Get(ctx, accountCollectCircuitCacheKey(tgAccountId))
		if err == nil && value != nil && !value.IsNil() {
			_ = value.Scan(&state)
		}
	}()
	if !state.permanent && !state.blockedUntil.IsZero() && time.Until(state.blockedUntil) <= 0 {
		return
	}
	if state.status != "" && state.status != "ready" {
		s.accountCircuitMu.Lock()
		if _, exists := s.accountCircuits[tgAccountId]; !exists {
			s.accountCircuits[tgAccountId] = state
		}
		s.accountCircuitMu.Unlock()
	}
}

func (s *sSysPublish) persistAccountCollectCircuit(ctx context.Context, tgAccountId int64, state accountCollectCircuit) {
	if s == nil || tgAccountId <= 0 {
		return
	}
	func() {
		defer func() { _ = recover() }()
		if err := cache.Instance().Set(ctx, accountCollectCircuitCacheKey(tgAccountId), state, accountCollectCircuitCacheTTL); err != nil {
			g.Log().Warningf(ctx, "持久化TG账号熔断状态失败 tgAccountId:%d err:%+v", tgAccountId, err)
		}
	}()
}

func (s *sSysPublish) clearPersistedAccountCollectCircuit(ctx context.Context, tgAccountId int64) {
	if s == nil || tgAccountId <= 0 {
		return
	}
	func() {
		defer func() { _ = recover() }()
		if _, err := cache.Instance().Remove(ctx, accountCollectCircuitCacheKey(tgAccountId)); err != nil {
			g.Log().Warningf(ctx, "清理TG账号熔断状态失败 tgAccountId:%d err:%+v", tgAccountId, err)
		}
	}()
}

func (s *sSysPublish) accountCollectCircuitState(tgAccountId int64) (accountCollectCircuit, bool) {
	if s == nil || tgAccountId <= 0 {
		return accountCollectCircuit{}, false
	}
	s.accountCircuitMu.Lock()
	defer s.accountCircuitMu.Unlock()
	state, ok := s.accountCircuits[tgAccountId]
	return state, ok
}

func (s *sSysPublish) accountCollectCircuitBlocked(tgAccountId int64) (time.Duration, bool) {
	state, ok := s.accountCollectCircuitState(tgAccountId)
	if !ok {
		return 0, false
	}
	if state.permanent {
		return 0, true
	}
	remaining := time.Until(state.blockedUntil)
	if remaining <= 0 {
		return 0, false
	}
	return remaining, true
}

func (s *sSysPublish) openAccountCollectCircuit(ctx context.Context, tgAccountId int64, reason error) time.Duration {
	if s == nil || tgAccountId <= 0 {
		return accountCollectCircuitInitialDelay
	}
	message := strings.TrimSpace(fmt.Sprint(reason))
	s.accountCircuitMu.Lock()
	state := s.accountCircuits[tgAccountId]
	state.failures++
	state.lastMessage = message
	state.updatedAt = time.Now()
	if isTelegramPermanentAccountAuthError(reason) || strings.Contains(strings.ToLower(message), "auth_bytes_invalid") {
		state.permanent = true
		state.status = "reauth_required"
		state.blockedUntil = time.Time{}
	} else {
		state.status = "reconnecting"
		exponent := math.Min(float64(state.failures-1), 6)
		delay := time.Duration(float64(accountCollectCircuitInitialDelay) * math.Pow(2, exponent))
		if delay > accountCollectCircuitMaxDelay {
			delay = accountCollectCircuitMaxDelay
		}
		state.blockedUntil = time.Now().Add(delay)
	}
	previous := s.accountCircuits[tgAccountId]
	s.accountCircuits[tgAccountId] = state
	s.accountCircuitMu.Unlock()
	s.persistAccountCollectCircuit(ctx, tgAccountId, state)

	if state.permanent {
		if !previous.permanent {
			g.Log().Errorf(ctx, "TG账号连接已熔断，需重新授权 tgAccountId:%d err:%s", tgAccountId, message)
		}
		return 0
	}
	delay := time.Until(state.blockedUntil)
	if delay < time.Second {
		delay = time.Second
	}
	if previous.blockedUntil.Before(time.Now()) || previous.failures == 0 {
		g.Log().Warningf(ctx, "TG账号连接进入熔断等待 tgAccountId:%d retryAfter:%s failures:%d err:%s", tgAccountId, delay.Round(time.Second), state.failures, message)
	}
	return delay
}

func (s *sSysPublish) closeAccountCollectCircuit(tgAccountId int64) {
	if s == nil || tgAccountId <= 0 {
		return
	}
	s.accountCircuitMu.Lock()
	state := s.accountCircuits[tgAccountId]
	if state.failures > 0 {
		state.recoveries++
		state.recoveredAt = time.Now()
	}
	state.status = "ready"
	state.permanent = false
	state.blockedUntil = time.Time{}
	state.updatedAt = time.Now()
	delete(s.accountCircuits, tgAccountId)
	s.accountCircuitMu.Unlock()
	if state.failures > 0 {
		g.Log().Infof(context.Background(), "TG账号连接恢复 tgAccountId:%d failures:%d recoveries:%d", tgAccountId, state.failures, state.recoveries)
	}
	s.persistAccountCollectCircuit(context.Background(), tgAccountId, state)
}

func (s *sSysPublish) accountCollectCircuitError(tgAccountId int64) error {
	delay, blocked := s.accountCollectCircuitBlocked(tgAccountId)
	if !blocked {
		return nil
	}
	state, _ := s.accountCollectCircuitState(tgAccountId)
	if state.permanent {
		return fmt.Errorf("TG账号需要重新授权，已暂停媒体任务 tgAccountId:%d", tgAccountId)
	}
	return &collectMediaRetryError{
		message: fmt.Sprintf("TG账号暂不可用，账号级熔断等待%s后自动恢复 tgAccountId:%d", delay.Round(time.Second), tgAccountId),
		delay:   delay,
	}
}

func (s *sSysPublish) accountCollectCircuitShouldStart(tgAccountId int64) bool {
	s.restoreAccountCollectCircuit(context.Background(), tgAccountId)
	_, blocked := s.accountCollectCircuitBlocked(tgAccountId)
	return !blocked
}

func collectMediaShouldReconnectAccount(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, pattern := range []string{
		"auth_bytes_invalid",
		"dc is closed",
		"connection reset",
		"connection refused",
		"connection closed",
		"broken pipe",
		"eof",
		"file_migrate",
	} {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}
