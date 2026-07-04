package sys

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	bda "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/bda/v20200324"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	iai "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/iai/v20200303"
)

type tencentVisionClient struct {
	secretId    string
	secretKey   string
	cloudSite   string
	region      string
	bdaEndpoint string
	iaiEndpoint string
}

type tencentVisionResult struct {
	FaceRaw    string
	SegmentRaw string
	FaceCount  int
}

func newTencentVisionClient(confSecretId string, confSecretKey string, cloudSite string, region string, bdaEndpoint string, iaiEndpoint string) *tencentVisionClient {
	cloudSite = strings.TrimSpace(cloudSite)
	if cloudSite == "" {
		cloudSite = "mainland"
	}
	region = strings.TrimSpace(region)
	iaiEndpoint = strings.TrimSpace(iaiEndpoint)
	if cloudSite == "intl" {
		// 国际版 IAI SDK 仍要求 Region 必传，空值会在请求前被 SDK 拦截。
		if region == "" || region == "ap-guangzhou" {
			region = "ap-singapore"
		}
		if iaiEndpoint == "" || iaiEndpoint == "iai.tencentcloudapi.com" {
			iaiEndpoint = "iai.intl.tencentcloudapi.com"
		}
	} else {
		if region == "" {
			region = "ap-guangzhou"
		}
		if iaiEndpoint == "" || iaiEndpoint == "iai.intl.tencentcloudapi.com" {
			iaiEndpoint = "iai.tencentcloudapi.com"
		}
	}
	return &tencentVisionClient{
		secretId:    strings.TrimSpace(confSecretId),
		secretKey:   strings.TrimSpace(confSecretKey),
		cloudSite:   cloudSite,
		region:      region,
		bdaEndpoint: strings.TrimSpace(bdaEndpoint),
		iaiEndpoint: iaiEndpoint,
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

func normalizeTencentVisionImageBytes(imageBytes []byte) ([]byte, error) {
	_, format, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, gerror.New("图片格式不支持，请上传 JPG、PNG、GIF 或 WEBP")
	}
	switch strings.ToLower(format) {
	case "jpeg", "png":
		return imageBytes, nil
	}
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, gerror.New("图片格式不支持，请上传 JPG、PNG、GIF 或 WEBP")
	}
	buf := bytes.NewBuffer(nil)
	if err = jpeg.Encode(buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, gerror.Wrap(err, "转换腾讯云检测图片失败")
	}
	return buf.Bytes(), nil
}

// detectFace 调用腾讯云人脸检测，结果用于二维码和贴图避开人脸区域。
func (c *tencentVisionClient) detectFace(ctx context.Context, imageBase64 string) (string, int, error) {
	if c.cloudSite == "intl" {
		return c.detectFaceIntl(ctx, imageBase64)
	}
	return c.detectFaceMainland(ctx, imageBase64)
}

func (c *tencentVisionClient) detectFaceMainland(ctx context.Context, imageBase64 string) (string, int, error) {
	_ = ctx
	client, err := c.newIaiClient()
	if err != nil {
		return "", 0, err
	}
	req := iai.NewDetectFaceRequest()
	req.Image = common.StringPtr(imageBase64)
	req.MaxFaceNum = common.Uint64Ptr(20)
	req.NeedFaceAttributes = common.Uint64Ptr(0)
	req.NeedQualityDetection = common.Uint64Ptr(0)
	req.NeedRotateDetection = common.Uint64Ptr(1)
	resp, err := client.DetectFace(req)
	if err != nil {
		return "", 0, wrapTencentSDKError(err, "调用腾讯云人脸检测失败")
	}
	raw := resp.ToJsonString()
	count := countTencentFaces(raw)
	return raw, count, nil
}

func (c *tencentVisionClient) detectFaceIntl(ctx context.Context, imageBase64 string) (string, int, error) {
	_ = ctx
	client, err := c.newIaiClient()
	if err != nil {
		return "", 0, err
	}
	req := iai.NewDetectFaceAttributesRequest()
	req.Image = common.StringPtr(imageBase64)
	req.MaxFaceNum = common.Uint64Ptr(20)
	req.FaceAttributesType = common.StringPtr("None")
	req.FaceModelVersion = common.StringPtr("3.0")
	req.NeedRotateDetection = common.Uint64Ptr(1)
	resp, err := client.DetectFaceAttributes(req)
	if err != nil {
		return "", 0, wrapTencentSDKError(err, "调用腾讯云国际版人脸检测失败")
	}
	raw := normalizeTencentIntlFaceRaw(resp.ToJsonString())
	count := countTencentFaces(raw)
	return raw, count, nil
}

func normalizeTencentIntlFaceRaw(raw string) string {
	var parsed struct {
		Response struct {
			ImageWidth      uint64 `json:"ImageWidth"`
			ImageHeight     uint64 `json:"ImageHeight"`
			FaceDetailInfos []struct {
				FaceRect struct {
					X      int64  `json:"X"`
					Y      int64  `json:"Y"`
					Width  uint64 `json:"Width"`
					Height uint64 `json:"Height"`
				} `json:"FaceRect"`
			} `json:"FaceDetailInfos"`
			RequestId string `json:"RequestId"`
		} `json:"Response"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw
	}
	faceInfos := make([]map[string]int64, 0, len(parsed.Response.FaceDetailInfos))
	for _, item := range parsed.Response.FaceDetailInfos {
		if item.FaceRect.Width == 0 || item.FaceRect.Height == 0 {
			continue
		}
		faceInfos = append(faceInfos, map[string]int64{
			"X":      item.FaceRect.X,
			"Y":      item.FaceRect.Y,
			"Width":  int64(item.FaceRect.Width),
			"Height": int64(item.FaceRect.Height),
		})
	}
	data, err := json.Marshal(map[string]interface{}{
		"Provider": "tencent-intl",
		"Response": map[string]interface{}{
			"ImageWidth":  parsed.Response.ImageWidth,
			"ImageHeight": parsed.Response.ImageHeight,
			"FaceInfos":   faceInfos,
			"RequestId":   parsed.Response.RequestId,
		},
	})
	if err != nil {
		return raw
	}
	return string(data)
}

// segmentPortrait 调用腾讯云 BDA SegmentPortraitPic，结果用于人像背景替换。
func (c *tencentVisionClient) segmentPortrait(ctx context.Context, imageBase64 string) (string, error) {
	_ = ctx
	client, err := c.newBdaClient()
	if err != nil {
		return "", err
	}
	req := bda.NewSegmentPortraitPicRequest()
	req.Image = common.StringPtr(imageBase64)
	req.RspImgType = common.StringPtr("base64")
	req.Scene = common.StringPtr("GEN")
	resp, err := client.SegmentPortraitPic(req)
	if err != nil {
		return "", wrapTencentSDKError(err, "调用腾讯云人像分割失败")
	}
	return resp.ToJsonString(), nil
}

func (c *tencentVisionClient) newIaiClient() (*iai.Client, error) {
	client, err := iai.NewClient(c.credential(), c.region, c.clientProfile(c.iaiEndpoint))
	if err != nil {
		return nil, gerror.Wrap(err, "初始化腾讯云人脸识别客户端失败")
	}
	return client, nil
}

func (c *tencentVisionClient) newBdaClient() (*bda.Client, error) {
	client, err := bda.NewClient(c.credential(), c.region, c.clientProfile(c.bdaEndpoint))
	if err != nil {
		return nil, gerror.Wrap(err, "初始化腾讯云人体分析客户端失败")
	}
	return client, nil
}

func (c *tencentVisionClient) credential() *common.Credential {
	return common.NewCredential(c.secretId, c.secretKey)
}

func (c *tencentVisionClient) clientProfile(endpoint string) *profile.ClientProfile {
	cpf := profile.NewClientProfile()
	cpf.SignMethod = "TC3-HMAC-SHA256"
	cpf.HttpProfile = profile.NewHttpProfile()
	cpf.HttpProfile.Endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	cpf.HttpProfile.ReqMethod = "POST"
	cpf.HttpProfile.ReqTimeout = 20
	return cpf
}

func wrapTencentSDKError(err error, prefix string) error {
	if sdkErr, ok := err.(*tcerr.TencentCloudSDKError); ok {
		if sdkErr.GetRequestId() == "" {
			return gerror.Newf("%s: %s %s", prefix, sdkErr.GetCode(), sdkErr.GetMessage())
		}
		return gerror.Newf("%s: %s %s requestId=%s", prefix, sdkErr.GetCode(), sdkErr.GetMessage(), sdkErr.GetRequestId())
	}
	return gerror.Wrap(err, prefix)
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
