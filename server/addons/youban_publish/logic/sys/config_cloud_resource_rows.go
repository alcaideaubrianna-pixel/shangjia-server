package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_publish/global"
	"hotgo/internal/dao"
)

func (s *sSysConfig) ensureCloudResourceConfigRows(ctx context.Context) error {
	rows := []g.Map{
		{"name": "OCR 开关", "type": "int", "key": "ocrEnabled", "value": "0", "default_value": "0", "sort": 10, "tip": "仅在开启背景替换时用于文档识别"},
		{"name": "OCR 服务地址", "type": "string", "key": "ocrEndpoint", "value": "", "default_value": "", "sort": 20, "tip": "OCR HTTP 服务地址，关闭 OCR 时可为空"},
		{"name": "OCR 超时时间", "type": "number", "key": "ocrTimeoutMs", "value": "800", "default_value": "800", "sort": 25, "tip": "OCR 请求超时时间，单位毫秒"},
		{"name": "OCR 屏蔽关键字", "type": "textarea", "key": "ocrBlockedKeywords", "value": "", "default_value": "", "sort": 30, "tip": "每行一个关键字，命中任意关键字时跳过背景替换"},
		{"name": "人像抠图来源", "type": "string", "key": "mattingProvider", "value": "facepp", "default_value": "facepp", "sort": 40, "tip": "aliyun/tencent/fapihub/facepp"},
		{"name": "阿里云 AccessKey ID", "type": "string", "key": "aliyunAccessKeyId", "value": "", "default_value": "", "sort": 50, "tip": "阿里云视觉智能开放平台 AccessKey ID"},
		{"name": "阿里云 AccessKey Secret", "type": "string", "key": "aliyunAccessKeySecret", "value": "", "default_value": "", "sort": 60, "tip": "页面回显会脱敏"},
		{"name": "阿里云 Endpoint", "type": "string", "key": "aliyunEndpoint", "value": "imageseg.cn-shanghai.aliyuncs.com", "default_value": "imageseg.cn-shanghai.aliyuncs.com", "sort": 70, "tip": "SegmentBody 上海地域 Endpoint"},
		{"name": "腾讯抠图处理桶", "type": "string", "key": "tencentMattingBucket", "value": "", "default_value": "", "sort": 80, "tip": "A 账号已开通数据万象的桶，格式 BucketName-APPID"},
		{"name": "腾讯抠图临时目录", "type": "string", "key": "tencentMattingPath", "value": "youban-matting", "default_value": "youban-matting", "sort": 90, "tip": "A 账号处理桶内的临时对象目录"},
		{"name": "腾讯视觉开关", "type": "int", "key": "tencentVisionEnabled", "value": "0", "default_value": "0", "sort": 92, "tip": "是否启用腾讯视觉能力"},
		{"name": "腾讯云站点", "type": "string", "key": "tencentCloudSite", "value": "intl", "default_value": "intl", "sort": 93, "tip": "腾讯云站点区域"},
		{"name": "腾讯 SecretId", "type": "string", "key": "tencentSecretId", "value": "", "default_value": "", "sort": 94, "tip": "页面回显会脱敏"},
		{"name": "腾讯 SecretKey", "type": "string", "key": "tencentSecretKey", "value": "", "default_value": "", "sort": 95, "tip": "页面回显会脱敏"},
		{"name": "腾讯 Region", "type": "string", "key": "tencentRegion", "value": "ap-singapore", "default_value": "ap-singapore", "sort": 96, "tip": "腾讯云地域"},
		{"name": "腾讯 BDA Endpoint", "type": "string", "key": "tencentBdaEndpoint", "value": "bda.intl.tencentcloudapi.com", "default_value": "bda.intl.tencentcloudapi.com", "sort": 97, "tip": "腾讯人体分析接口地址"},
		{"name": "腾讯 IAI Endpoint", "type": "string", "key": "tencentIaiEndpoint", "value": "iai.intl.tencentcloudapi.com", "default_value": "iai.intl.tencentcloudapi.com", "sort": 98, "tip": "腾讯人脸接口地址"},
		{"name": "FAPIHub 开关", "type": "int", "key": "fapiHubEnabled", "value": "0", "default_value": "0", "sort": 99, "tip": "是否启用 FAPIHub"},
		{"name": "FAPIHub API Key", "type": "string", "key": "fapiHubApiKey", "value": "", "default_value": "", "sort": 101, "tip": "页面回显会脱敏"},
		{"name": "FAPIHub Endpoint", "type": "string", "key": "fapiHubEndpoint", "value": "https://fapihub.com/v2/rembg/", "default_value": "https://fapihub.com/v2/rembg/", "sort": 102, "tip": "FAPIHub 抠图接口地址"},
		{"name": "FAPIHub Model", "type": "string", "key": "fapiHubModel", "value": "falcon", "default_value": "falcon", "sort": 103, "tip": "FAPIHub 模型名称"},
		{"name": "Face++ API Key", "type": "string", "key": "facePlusApiKey", "value": "", "default_value": "", "sort": 100, "tip": "Face++ 人体抠像 API Key"},
		{"name": "Face++ API Secret", "type": "string", "key": "facePlusApiSecret", "value": "", "default_value": "", "sort": 110, "tip": "页面回显会脱敏"},
		{"name": "Face++ Endpoint", "type": "string", "key": "facePlusEndpoint", "value": "https://api-cn.faceplusplus.com/humanbodypp/v2/segment", "default_value": "https://api-cn.faceplusplus.com/humanbodypp/v2/segment", "sort": 120, "tip": "Face++ 人体抠像接口地址"},
		{"name": "Face++ 并发数", "type": "number", "key": "facePlusConcurrency", "value": "2", "default_value": "2", "sort": 130, "tip": "Face++ API 并发限制"},
	}
	for _, row := range rows {
		key := gconv.String(row["key"])
		count, err := dao.SysAddonsConfig.Ctx(ctx).
			Where("addon_name", global.GetSkeleton().Name).
			Where("group", publishConfigGroupCloudResource).
			Where("key", key).
			Count()
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		fillCloudResourceConfigRow(row)
		if _, err = dao.SysAddonsConfig.Ctx(ctx).Data(row).Insert(); err != nil {
			return err
		}
	}
	return nil
}

func fillCloudResourceConfigRow(row g.Map) {
	now := gtime.Now()
	row["addon_name"] = global.GetSkeleton().Name
	row["group"] = publishConfigGroupCloudResource
	row["is_default"] = 1
	row["status"] = 1
	row["created_at"] = now
	row["updated_at"] = now
}
