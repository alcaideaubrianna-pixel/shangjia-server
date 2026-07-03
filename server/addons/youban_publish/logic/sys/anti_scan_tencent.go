package sys

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const tencentCloudAlgorithm = "TC3-HMAC-SHA256"

type tencentVisionClient struct {
	secretId    string
	secretKey   string
	region      string
	bdaEndpoint string
	iaiEndpoint string
}

type tencentVisionResult struct {
	FaceRaw    string
	SegmentRaw string
	FaceCount  int
}

func newTencentVisionClient(confSecretId string, confSecretKey string, region string, bdaEndpoint string, iaiEndpoint string) *tencentVisionClient {
	return &tencentVisionClient{
		secretId:    strings.TrimSpace(confSecretId),
		secretKey:   strings.TrimSpace(confSecretKey),
		region:      strings.TrimSpace(region),
		bdaEndpoint: strings.TrimSpace(bdaEndpoint),
		iaiEndpoint: strings.TrimSpace(iaiEndpoint),
	}
}

func (c *tencentVisionClient) detect(ctx context.Context, imageBase64 string) (*tencentVisionResult, error) {
	if c.secretId == "" || c.secretKey == "" {
		return nil, gerror.New("腾讯云视觉密钥未配置")
	}
	faceRaw, faceCount, faceErr := c.detectFace(ctx, imageBase64)
	if faceErr != nil {
		return nil, faceErr
	}
	segmentRaw, segmentErr := c.segmentPortrait(ctx, imageBase64)
	if segmentErr != nil {
		return nil, segmentErr
	}
	return &tencentVisionResult{
		FaceRaw:    faceRaw,
		SegmentRaw: segmentRaw,
		FaceCount:  faceCount,
	}, nil
}

// detectFace 调用腾讯云 IAI DetectFace，结果用于二维码和贴图避开人脸区域。
func (c *tencentVisionClient) detectFace(ctx context.Context, imageBase64 string) (string, int, error) {
	body := g.Map{
		"Image":                imageBase64,
		"NeedFaceAttributes":   0,
		"NeedQualityDetection": 0,
	}
	raw, err := c.call(ctx, c.iaiEndpoint, "iai", "DetectFace", "2020-03-03", body)
	if err != nil {
		return "", 0, err
	}
	count := countTencentFaces(raw)
	return raw, count, nil
}

// segmentPortrait 调用腾讯云 BDA SegmentPortraitPic，结果用于人像背景替换。
func (c *tencentVisionClient) segmentPortrait(ctx context.Context, imageBase64 string) (string, error) {
	body := g.Map{"Image": imageBase64}
	return c.call(ctx, c.bdaEndpoint, "bda", "SegmentPortraitPic", "2020-03-24", body)
}

// call 使用腾讯云 TC3-HMAC-SHA256 签名发起视觉 API 请求，避免引入额外 SDK。
func (c *tencentVisionClient) call(ctx context.Context, endpoint string, service string, action string, version string, body g.Map) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", gerror.Wrap(err, "编码腾讯云请求失败")
	}
	timestamp := time.Now().Unix()
	auth := c.authorization(endpoint, service, action, payload, timestamp)
	resp, err := g.Client().
		SetHeader("Authorization", auth).
		SetHeader("Content-Type", "application/json; charset=utf-8").
		SetHeader("Host", endpoint).
		SetHeader("X-TC-Action", action).
		SetHeader("X-TC-Version", version).
		SetHeader("X-TC-Timestamp", strconv.FormatInt(timestamp, 10)).
		SetHeader("X-TC-Region", c.region).
		SetTimeout(20*time.Second).
		Post(ctx, "https://"+endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", gerror.Wrap(err, "请求腾讯云视觉接口失败")
	}
	defer resp.Close()
	raw := string(resp.ReadAll())
	if resp.StatusCode != 200 {
		return "", gerror.Newf("腾讯云视觉接口 HTTP %d: %s", resp.StatusCode, raw)
	}
	if strings.Contains(raw, `"Error"`) {
		return "", gerror.Newf("腾讯云视觉接口返回错误: %s", raw)
	}
	return raw, nil
}

func (c *tencentVisionClient) authorization(endpoint string, service string, action string, payload []byte, timestamp int64) string {
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	hashedPayload := sha256Hex(payload)
	canonicalHeaders := fmt.Sprintf("content-type:application/json; charset=utf-8\nhost:%s\nx-tc-action:%s\n", endpoint, strings.ToLower(action))
	signedHeaders := "content-type;host;x-tc-action"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + hashedPayload
	credentialScope := date + "/" + service + "/tc3_request"
	stringToSign := tencentCloudAlgorithm + "\n" + strconv.FormatInt(timestamp, 10) + "\n" + credentialScope + "\n" + sha256Hex([]byte(canonicalRequest))
	secretDate := hmacSHA256([]byte("TC3"+c.secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	return fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s", tencentCloudAlgorithm, c.secretId, credentialScope, signedHeaders, signature)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, msg string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}

func countTencentFaces(raw string) int {
	var parsed struct {
		Response struct {
			FaceInfos []interface{} `json:"FaceInfos"`
		} `json:"Response"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return 0
	}
	return len(parsed.Response.FaceInfos)
}
