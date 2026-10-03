package epay

import (
	"net/url"
	"strings"
	"testing"

	"hotgo/internal/model"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/payin"
)

func TestCreateOrderUsesXcashEPayV1Parameters(t *testing.T) {
	driver := New(&model.PayConfig{
		RainbowGateway: "https://xcash.example/epay",
		RainbowPid:     "1001",
		RainbowKey:     "secret",
	})

	res, err := driver.CreateOrder(t.Context(), payin.CreateOrderInp{Pay: &entity.PayLog{
		OutTradeNo: "order-001",
		NotifyUrl:  "https://merchant.example/notify",
		ReturnUrl:  "https://merchant.example/return",
		Subject:    "Premium Plan",
		PayAmount:  29.99,
	}})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	payURL, err := url.Parse(res.PayURL)
	if err != nil {
		t.Fatalf("parse pay URL: %v", err)
	}
	if payURL.Path != "/epay/submit.php" {
		t.Fatalf("gateway path = %q, want /epay/submit.php", payURL.Path)
	}
	params := make(map[string]string)
	for key, values := range payURL.Query() {
		params[key] = values[0]
	}
	if params["currency"] != defaultCurrency || res.Currency != defaultCurrency {
		t.Fatalf("currency params/result = %q/%q, want %s", params["currency"], res.Currency, defaultCurrency)
	}
	if params["sign_type"] != signTypeMD5 {
		t.Fatalf("sign_type = %q, want %s", params["sign_type"], signTypeMD5)
	}
	if params["sign"] != signParams(params, "secret") {
		t.Fatalf("sign = %q, want valid EPay MD5 signature", params["sign"])
	}
}

func TestBuildSignContentMatchesXcashEPayV1(t *testing.T) {
	params := map[string]string{
		"sign":         "ignored",
		"sign_type":    "MD5",
		"return_url":   "",
		"pid":          "1001",
		"out_trade_no": "order-001",
		"currency":     "USD",
	}

	got := buildSignContent(params)
	want := "currency=USD&out_trade_no=order-001&pid=1001"
	if got != want {
		t.Fatalf("buildSignContent() = %q, want %q", got, want)
	}
	if strings.ToLower(signParams(params, "secret")) != signParams(params, "secret") {
		t.Fatal("EPay MD5 signature must use lowercase hexadecimal")
	}
}
