// Package rainbow 彩虹易支付
package rainbow

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gfile"
)

func signParams(params map[string]string, privateKeyText string) (string, error) {
	privateKey, err := parsePrivateKey(loadKeyText(privateKeyText))
	if err != nil {
		return "", err
	}

	digest := sha256.Sum256([]byte(buildSignContent(params)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", gerror.Wrap(err, "彩虹易支付签名失败")
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func verifyParams(params map[string]string, publicKeyText string) error {
	sign := params["sign"]
	if sign == "" {
		return gerror.New("彩虹易支付回调缺少签名")
	}

	publicKey, err := parsePublicKey(loadKeyText(publicKeyText))
	if err != nil {
		return err
	}

	signature, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return gerror.Wrap(err, "彩虹易支付签名格式错误")
	}

	digest := sha256.Sum256([]byte(buildSignContent(params)))
	if err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return gerror.Wrap(err, "彩虹易支付验签不通过")
	}
	return nil
}

func buildSignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "sign" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	return strings.Join(parts, "&")
}

func loadKeyText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "-----BEGIN") {
		return value
	}
	if gfile.Exists(value) {
		return gfile.GetContents(value)
	}
	return value
}

func parsePrivateKey(text string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(text))
	if block == nil {
		return nil, gerror.New("彩虹易支付商户私钥格式错误")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, gerror.Wrap(err, "解析彩虹易支付商户私钥失败")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, gerror.New("彩虹易支付商户私钥不是 RSA 私钥")
	}
	return key, nil
}

func parsePublicKey(text string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(text))
	if block == nil {
		return nil, gerror.New("彩虹易支付平台公钥格式错误")
	}

	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		if key, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return key, nil
		}
		return nil, gerror.New("彩虹易支付平台证书不是 RSA 公钥")
	}

	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, gerror.Wrap(err, "解析彩虹易支付平台公钥失败")
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, gerror.New("彩虹易支付平台公钥不是 RSA 公钥")
	}
	return key, nil
}
