package sys

import "testing"

func TestTenantVipPaymentReturnURLReplacesOrderPlaceholder(t *testing.T) {
	got := tenantVipPaymentReturnURL("https://merchant.test/#/admin/vip/result?orderNo=__ORDER_NO__", "YBPVIP123")
	want := "https://merchant.test/#/admin/vip/result?orderNo=YBPVIP123"
	if got != want {
		t.Fatalf("return URL = %q, want %q", got, want)
	}
}

func TestTenantVipPaymentReturnURLSupportsHashRouter(t *testing.T) {
	got := tenantVipPaymentReturnURL("https://merchant.test/#/admin/vip/result", "YBPVIP123")
	want := "https://merchant.test/#/admin/vip/result?orderNo=YBPVIP123"
	if got != want {
		t.Fatalf("return URL = %q, want %q", got, want)
	}
}

func TestTenantVipPaymentReturnURLSupportsHistoryRouter(t *testing.T) {
	got := tenantVipPaymentReturnURL("https://merchant.test/admin/vip/result", "YBPVIP123")
	want := "https://merchant.test/admin/vip/result?orderNo=YBPVIP123"
	if got != want {
		t.Fatalf("return URL = %q, want %q", got, want)
	}
}
