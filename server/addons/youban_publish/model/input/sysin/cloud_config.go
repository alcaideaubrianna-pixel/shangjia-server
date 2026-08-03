package sysin

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model"
	"hotgo/internal/model/input/form"
)

const (
	CloudResourceTypeBackgroundMatting = "background_matting"
	CloudResourceTypeFaceDetection     = "face_detection"
)

type CloudResourceConfigViewInp struct{}

type CloudResourceConfigViewModel struct {
	*model.CloudResourceConfig
}

type CloudResourceConfigSaveInp struct {
	model.CloudResourceConfig
}

type CloudResourceUsageListInp struct {
	form.PageReq
	StartDate    string `json:"startDate" dc:"开始日期，格式 YYYY-MM-DD"`
	EndDate      string `json:"endDate" dc:"结束日期，格式 YYYY-MM-DD"`
	ResourceType string `json:"resourceType" dc:"资源类型"`
	Keyword      string `json:"keyword" dc:"账号、昵称、账号归属"`
}

type CloudResourceUsageModel struct {
	AccountId              int64  `json:"accountId" dc:"账号ID，0为系统调用"`
	TenantId               int64  `json:"tenantId" dc:"账号归属ID"`
	Username               string `json:"username" dc:"用户名"`
	Nickname               string `json:"nickname" dc:"昵称"`
	TenantName             string `json:"tenantName" dc:"账号归属"`
	VipLevel               int    `json:"vipLevel" dc:"会员等级"`
	VipStatus              int    `json:"vipStatus" dc:"会员状态"`
	VipExpiredAt           string `json:"vipExpiredAt" dc:"会员到期时间"`
	RequestCount           int64  `json:"requestCount" dc:"请求次数"`
	SuccessCount           int64  `json:"successCount" dc:"成功次数"`
	FailureCount           int64  `json:"failureCount" dc:"失败次数"`
	BackgroundMattingCount int64  `json:"backgroundMattingCount" dc:"云端抠图次数"`
	FaceDetectionCount     int64  `json:"faceDetectionCount" dc:"人脸检测次数"`
	ValidationCount        int64  `json:"validationCount" dc:"配置验证次数"`
	TotalDurationMs        int64  `json:"totalDurationMs" dc:"累计耗时毫秒"`
	AvgDurationMs          int64  `json:"avgDurationMs" dc:"平均耗时毫秒"`
	FirstUsageDate         string `json:"firstUsageDate" dc:"首次调用日期"`
	LastCalledAt           string `json:"lastCalledAt" dc:"最后调用时间"`
}

type CloudResourceUsageSummaryModel struct {
	ActiveUserCount        int64 `json:"activeUserCount" dc:"调用用户数"`
	RequestCount           int64 `json:"requestCount" dc:"请求次数"`
	SuccessCount           int64 `json:"successCount" dc:"成功次数"`
	FailureCount           int64 `json:"failureCount" dc:"失败次数"`
	BackgroundMattingCount int64 `json:"backgroundMattingCount" dc:"云端抠图次数"`
	FaceDetectionCount     int64 `json:"faceDetectionCount" dc:"人脸检测次数"`
	ValidationCount        int64 `json:"validationCount" dc:"配置验证次数"`
	TotalDurationMs        int64 `json:"totalDurationMs" dc:"累计耗时毫秒"`
	AvgDurationMs          int64 `json:"avgDurationMs" dc:"平均耗时毫秒"`
}

func (in *CloudResourceUsageListInp) Filter(ctx context.Context) error {
	_ = ctx
	if in == nil {
		return gerror.New("云资源调用统计参数不能为空")
	}
	now := time.Now()
	if strings.TrimSpace(in.StartDate) == "" {
		in.StartDate = now.Format("2006-01") + "-01"
	}
	if strings.TrimSpace(in.EndDate) == "" {
		in.EndDate = now.Format("2006-01-02")
	}
	startDate, err := time.Parse("2006-01-02", in.StartDate)
	if err != nil {
		return gerror.New("开始日期格式不正确")
	}
	endDate, err := time.Parse("2006-01-02", in.EndDate)
	if err != nil {
		return gerror.New("结束日期格式不正确")
	}
	if startDate.After(endDate) {
		return gerror.New("开始日期不能晚于结束日期")
	}
	if endDate.Sub(startDate) > 366*24*time.Hour {
		return gerror.New("单次查询时间范围不能超过一年")
	}
	in.Keyword = strings.TrimSpace(in.Keyword)
	in.ResourceType = strings.TrimSpace(in.ResourceType)
	if in.ResourceType != "" && in.ResourceType != CloudResourceTypeBackgroundMatting && in.ResourceType != CloudResourceTypeFaceDetection {
		return gerror.New("云资源类型不合法")
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 20
	}
	if in.PerPage > 100 {
		in.PerPage = 100
	}
	in.Pagination = true
	return nil
}

func (in *CloudResourceConfigSaveInp) Filter(ctx context.Context) error {
	in.TencentSecretId = strings.TrimSpace(in.TencentSecretId)
	in.TencentSecretKey = strings.TrimSpace(in.TencentSecretKey)
	in.TencentCloudSite = strings.TrimSpace(in.TencentCloudSite)
	in.TencentRegion = strings.TrimSpace(in.TencentRegion)
	in.TencentBdaEndpoint = strings.TrimSpace(in.TencentBdaEndpoint)
	in.TencentIaiEndpoint = strings.TrimSpace(in.TencentIaiEndpoint)
	in.FapiHubApiKey = strings.TrimSpace(in.FapiHubApiKey)
	in.FapiHubEndpoint = strings.TrimSpace(in.FapiHubEndpoint)
	in.FapiHubModel = strings.TrimSpace(in.FapiHubModel)
	if err := checkSwitch(in.TencentVisionEnabled, "腾讯云视觉开关"); err != nil {
		return err
	}
	// 人脸检测已从防扫图链路停用，保留字段仅用于兼容历史配置。
	in.TencentVisionEnabled = 0
	if err := checkSwitch(in.FapiHubEnabled, "FAPIHub 抠图开关"); err != nil {
		return err
	}
	if in.TencentCloudSite == "" {
		in.TencentCloudSite = "mainland"
	}
	if in.TencentCloudSite != "mainland" && in.TencentCloudSite != "intl" {
		return gerror.New("腾讯云站点不合法")
	}
	if in.TencentCloudSite == "intl" && (in.TencentRegion == "" || in.TencentRegion == "ap-guangzhou") {
		in.TencentRegion = "ap-singapore"
	}
	if in.TencentCloudSite == "mainland" && in.TencentRegion == "" {
		in.TencentRegion = "ap-guangzhou"
	}
	if in.TencentBdaEndpoint == "" {
		in.TencentBdaEndpoint = "bda.tencentcloudapi.com"
	}
	if in.TencentCloudSite == "intl" {
		if in.TencentIaiEndpoint == "" || in.TencentIaiEndpoint == "iai.tencentcloudapi.com" {
			in.TencentIaiEndpoint = "iai.intl.tencentcloudapi.com"
		}
	} else {
		if in.TencentIaiEndpoint == "" || in.TencentIaiEndpoint == "iai.intl.tencentcloudapi.com" {
			in.TencentIaiEndpoint = "iai.tencentcloudapi.com"
		}
	}
	if in.FapiHubEndpoint == "" {
		in.FapiHubEndpoint = "https://fapihub.com/v2/rembg/"
	}
	if in.FapiHubModel == "" {
		in.FapiHubModel = "falcon"
	}
	if in.FapiHubEnabled == 1 && in.FapiHubApiKey == "" {
		return gerror.New("启用 FAPIHub 抠图后必须配置 API Key")
	}
	return nil
}
