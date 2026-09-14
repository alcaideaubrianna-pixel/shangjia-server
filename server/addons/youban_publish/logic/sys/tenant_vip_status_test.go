package sys

import (
	"testing"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestNormalizeTenantVipStatusExpired(t *testing.T) {
	now := gtime.New("2026-09-14 20:00:00")
	status := &sysin.TenantVipStatusModel{
		IsVip:     true,
		Status:    consts.StatusEnabled,
		ExpiredAt: gtime.New("2026-09-14 19:16:04"),
		Features:  []string{sysin.TenantVipFeatureSimilarMedia},
	}

	got := normalizeTenantVipStatus(status, now)
	if got.IsVip || got.Status != consts.StatusDisable || len(got.Features) != 0 {
		t.Fatalf("expired status was not disabled: %+v", got)
	}
	if !status.IsVip || len(status.Features) != 1 {
		t.Fatal("normalization must not mutate the cached value")
	}
}

func TestNormalizeTenantVipStatusActive(t *testing.T) {
	now := gtime.New("2026-09-14 19:00:00")
	status := &sysin.TenantVipStatusModel{IsVip: true, ExpiredAt: gtime.New("2026-09-14 20:00:00")}
	if got := normalizeTenantVipStatus(status, now); got != status {
		t.Fatal("active status should be returned unchanged")
	}
}

func TestTenantVipStatusCacheTTLCappedByExpiration(t *testing.T) {
	now := gtime.New("2026-09-14 19:00:00")
	status := &sysin.TenantVipStatusModel{IsVip: true, ExpiredAt: gtime.New("2026-09-14 19:02:00")}
	if got := tenantVipStatusCacheTTL(status, 5*time.Minute, now); got != 2*time.Minute {
		t.Fatalf("cache TTL = %s, want 2m", got)
	}
	status.ExpiredAt = nil
	if got := tenantVipStatusCacheTTL(status, 5*time.Minute, now); got != 5*time.Minute {
		t.Fatalf("lifetime VIP cache TTL = %s, want 5m", got)
	}
}
