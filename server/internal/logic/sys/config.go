// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"fmt"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/global"
	"hotgo/internal/library/payment"
	"hotgo/internal/library/sms"
	"hotgo/internal/library/storager"
	"hotgo/internal/library/token"
	"hotgo/internal/library/wechat"
	"hotgo/internal/model"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
	"hotgo/utility/simple"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
)

type sSysConfig struct{}

func NewSysConfig() *sSysConfig {
	return &sSysConfig{}
}

func init() {
	service.RegisterSysConfig(NewSysConfig())
}

// InitConfig 初始化系统配置
func (s *sSysConfig) InitConfig(ctx context.Context) {
	if err := s.LoadConfig(ctx); err != nil {
		g.Log().Fatalf(ctx, "InitConfig fail：%+v", err)
	}
}

// LoadConfig 加载系统配置
func (s *sSysConfig) LoadConfig(ctx context.Context) (err error) {
	wx, err := s.GetWechat(ctx)
	if err != nil {
		return
	}
	wechat.SetConfig(wx)

	pay, err := s.GetPay(ctx)
	if err != nil {
		return
	}
	payment.SetConfig(pay)

	upload, err := s.GetUpload(ctx)
	if err != nil {
		return
	}
	storager.SetConfig(upload)

	sm, err := s.GetSms(ctx)
	if err != nil {
		return
	}
	sms.SetConfig(sm)

	tk, err := s.GetLoadToken(ctx)
	if err != nil {
		return
	}
	token.SetConfig(tk)

	// 更多
	// ...
	return
}

// GetLogin 获取登录配置
func (s *sSysConfig) GetLogin(ctx context.Context) (conf *model.LoginConfig, err error) {
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "login"})
	if err != nil {
		return
	}
	err = gconv.Scan(models.List, &conf)
	return
}

// GetWechat 获取微信配置
func (s *sSysConfig) GetWechat(ctx context.Context) (conf *model.WechatConfig, err error) {
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "wechat"})
	if err != nil {
		return
	}
	err = gconv.Scan(models.List, &conf)
	return
}

// GetPay 获取支付配置
func (s *sSysConfig) GetPay(ctx context.Context) (conf *model.PayConfig, err error) {
	if err = s.ensureEpayConfig(ctx); err != nil {
		return
	}
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "pay"})
	if err != nil {
		return
	}
	err = gconv.Scan(models.List, &conf)
	return
}

func (s *sSysConfig) ensureEpayConfig(ctx context.Context) (err error) {
	defaults := []struct {
		key   string
		name  string
		typ   string
		value interface{}
		sort  int
		tip   string
	}{
		{key: "payGMPayGateway", name: "GMPay 网关地址", typ: consts.ConfigTypeString, value: "http://127.0.0.1:18000", sort: 940, tip: "GMPay 网关地址"},
		{key: "payGMPayPid", name: "GMPay 商户ID", typ: consts.ConfigTypeString, value: "", sort: 950, tip: "GMPay 商户PID"},
		{key: "payGMPayKey", name: "GMPay Secret Key", typ: consts.ConfigTypeString, value: "", sort: 960, tip: "GMPay API secret_key，用于 HMAC-SHA256 签名"},
		{key: "payGMPayCurrency", name: "GMPay 结算币种", typ: consts.ConfigTypeString, value: "USD", sort: 970, tip: "三位计价币种；会员价格按U设置时使用USD，人民币价格使用CNY"},
		{key: "payGMPayToken", name: "GMPay 默认代币", typ: consts.ConfigTypeString, value: "", sort: 980, tip: "与 network 同时配置；两者留空时由结账页选择支付方式"},
		{key: "payGMPayNetwork", name: "GMPay 默认网络", typ: consts.ConfigTypeString, value: "", sort: 990, tip: "与 token 同时配置；两者留空时由结账页选择支付方式"},
		{key: "payRainbowGateway", name: "彩虹易支付网关地址", typ: consts.ConfigTypeString, value: "https://pay.v8jisu.cn", sort: 990, tip: "彩虹易支付网关地址"},
		{key: "payRainbowPid", name: "彩虹易支付商户ID", typ: consts.ConfigTypeString, value: "", sort: 1000, tip: "彩虹易支付商户ID"},
		{key: "payRainbowKey", name: "彩虹易支付MD5密钥", typ: consts.ConfigTypeString, value: "", sort: 1010, tip: "彩虹易支付 MD5 通讯密钥"},
	}

	cols := dao.SysConfig.Columns()
	var rows []*entity.SysConfig
	if err = dao.SysConfig.Ctx(ctx).Where(cols.Group, "pay").Scan(&rows); err != nil {
		return
	}
	exists := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		exists[row.Key] = struct{}{}
	}
	for _, item := range defaults {
		if _, ok := exists[item.key]; ok {
			continue
		}
		_, err = dao.SysConfig.Ctx(ctx).Data(g.Map{
			cols.Group:        "pay",
			cols.Key:          item.key,
			cols.Name:         item.name,
			cols.Type:         item.typ,
			cols.Value:        normalizeConfigValue(item.value),
			cols.DefaultValue: normalizeConfigValue(item.value),
			cols.IsDefault:    0,
			cols.Sort:         item.sort,
			cols.Tip:          item.tip,
			cols.Status:       consts.StatusEnabled,
			cols.CreatedAt:    gtime.Now(),
			cols.UpdatedAt:    gtime.Now(),
		}).Insert()
		if err != nil {
			return
		}
	}
	return
}

// GetMemberVip 获取会员认证配置
func (s *sSysConfig) GetMemberVip(ctx context.Context) (conf *model.MemberVipConfig, err error) {
	if err = s.ensureMemberVipConfig(ctx); err != nil {
		return
	}
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "member_vip"})
	if err != nil {
		return
	}
	err = gconv.Scan(models.List, &conf)
	if conf == nil {
		conf = defaultMemberVipConfig()
	}
	return
}

// GetSms 获取短信配置
func (s *sSysConfig) GetSms(ctx context.Context) (conf *model.SmsConfig, err error) {
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "sms"})
	if err != nil {
		return
	}
	err = gconv.Scan(models.List, &conf)
	return
}

// GetGeo 获取地理配置
func (s *sSysConfig) GetGeo(ctx context.Context) (conf *model.GeoConfig, err error) {
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "geo"})
	if err != nil {
		return
	}
	err = gconv.Scan(models.List, &conf)
	return
}

// GetUpload 获取上传配置
func (s *sSysConfig) GetUpload(ctx context.Context) (conf *model.UploadConfig, err error) {
	if err = s.ensureCosUploadConfig(ctx); err != nil {
		return
	}
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "upload"})
	if err != nil {
		return
	}
	err = gconv.Scan(models.List, &conf)
	if err == nil && conf != nil && conf.CosBucket != "" && conf.CosRegion != "" {
		conf.CosBucketURL = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", conf.CosBucket, conf.CosRegion)
	}
	if err == nil {
		conf, err = storager.ApplyUploadEndpointEnv(conf)
	}
	return
}

func (s *sSysConfig) ensureCosUploadConfig(ctx context.Context) (err error) {
	defaults := []struct {
		key  string
		name string
		sort int
		tip  string
	}{
		{key: "uploadCosBucket", name: "COS Bucket", sort: 475, tip: "存储桶名称，必须包含APPID后缀，例如：bucket-1250000000"},
		{key: "uploadCosRegion", name: "COS Region", sort: 476, tip: "存储桶所属地域，例如：ap-hongkong"},
		{key: "uploadR2AccessKey", name: "R2 Access Key ID", sort: 473, tip: "Cloudflare R2 Access Key ID"},
		{key: "uploadR2SecretKey", name: "R2 Secret Access Key", sort: 474, tip: "Cloudflare R2 Secret Access Key"},
		{key: "uploadR2Endpoint", name: "R2 Endpoint", sort: 477, tip: "Cloudflare R2 S3 API Endpoint"},
		{key: "uploadR2Bucket", name: "R2 Bucket", sort: 478, tip: "R2 存储桶名称"},
		{key: "uploadR2Region", name: "R2 Region", sort: 479, tip: "R2 通常使用 auto"},
		{key: "uploadR2PublicURL", name: "R2 公开访问域名", sort: 480, tip: "R2 自定义域名或公开访问地址"},
		{key: "uploadOssSecretId", name: "OSS AccessKey ID", sort: 481, tip: "阿里云 RAM 用户的 AccessKey ID"},
		{key: "uploadOssSecretKey", name: "OSS AccessKey Secret", sort: 482, tip: "阿里云 RAM 用户的 AccessKey Secret"},
		{key: "uploadOssEndpoint", name: "OSS 内网 Endpoint", sort: 483, tip: "SAE 同地域访问地址，例如：https://oss-ap-southeast-1-internal.aliyuncs.com"},
		{key: "uploadOssBucket", name: "OSS Bucket", sort: 484, tip: "OSS 存储空间名称"},
		{key: "uploadOssBucketURL", name: "OSS 公开访问域名", sort: 485, tip: "OSS 自定义域名或 Bucket 公网访问地址"},
		{key: "uploadOssPath", name: "OSS 存储路径", sort: 486, tip: "对象存储目录前缀，例如：hotgo/file"},
		{key: "uploadOssUploadURL", name: "OSS 浏览器上传域名", sort: 487, tip: "可选，浏览器直传使用的 OSS CNAME，例如：https://img.xiaohuiji.cc；留空时使用 OSS 公网 Endpoint"},
	}

	cols := dao.SysConfig.Columns()
	for _, item := range defaults {
		count, countErr := dao.SysConfig.Ctx(ctx).
			Where(cols.Group, "upload").
			Where(cols.Key, item.key).
			Count()
		if countErr != nil {
			return countErr
		}
		if count > 0 {
			continue
		}
		_, err = dao.SysConfig.Ctx(ctx).Data(g.Map{
			cols.Group:        "upload",
			cols.Key:          item.key,
			cols.Name:         item.name,
			cols.Type:         consts.ConfigTypeString,
			cols.Value:        "",
			cols.DefaultValue: "",
			cols.IsDefault:    0,
			cols.Sort:         item.sort,
			cols.Tip:          item.tip,
			cols.Status:       consts.StatusEnabled,
			cols.CreatedAt:    gtime.Now(),
			cols.UpdatedAt:    gtime.Now(),
		}).Insert()
		if err != nil {
			return
		}
	}
	return
}

// GetSmtp 获取邮件配置
func (s *sSysConfig) GetSmtp(ctx context.Context) (conf *model.EmailConfig, err error) {
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "smtp"})
	if err != nil {
		return
	}
	if err = gconv.Scan(models.List, &conf); err != nil {
		return
	}

	conf.Addr = fmt.Sprintf("%s:%d", conf.Host, conf.Port)

	return
}

// GetBasic 获取基础配置
func (s *sSysConfig) GetBasic(ctx context.Context) (conf *model.BasicConfig, err error) {
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "basic"})
	if err != nil {
		return
	}
	err = gconv.Scan(models.List, &conf)
	return
}

// GetLoadTCP 获取本地tcp配置
func (s *sSysConfig) GetLoadTCP(ctx context.Context) (conf *model.TCPConfig, err error) {
	err = g.Cfg().MustGet(ctx, "tcp").Scan(&conf)
	return
}

// GetLoadGenerate 获取本地生成配置
func (s *sSysConfig) GetLoadGenerate(ctx context.Context) (conf *model.GenerateConfig, err error) {
	err = g.Cfg().MustGet(ctx, "hggen").Scan(&conf)
	return
}

// GetLoadToken 获取本地token配置
func (s *sSysConfig) GetLoadToken(ctx context.Context) (conf *model.TokenConfig, err error) {
	err = g.Cfg().MustGet(ctx, "token").Scan(&conf)
	return
}

// GetLoadLog 获取本地日志配置
func (s *sSysConfig) GetLoadLog(ctx context.Context) (conf *model.LogConfig, err error) {
	err = g.Cfg().MustGet(ctx, "system.log").Scan(&conf)
	return
}

// GetLoadServeLog 获取本地服务日志配置
func (s *sSysConfig) GetLoadServeLog(ctx context.Context) (conf *model.ServeLogConfig, err error) {
	err = g.Cfg().MustGet(ctx, "system.serveLog").Scan(&conf)
	return
}

// GetConfigByGroup 获取指定分组的配置
func (s *sSysConfig) GetConfigByGroup(ctx context.Context, in *sysin.GetConfigInp) (res *sysin.GetConfigModel, err error) {
	if in.Group == "" {
		err = gerror.New("分组不能为空")
		return
	}
	if in.Group == "member_vip" {
		if err = s.ensureMemberVipConfig(ctx); err != nil {
			return
		}
	}
	if in.Group == "upload" {
		if err = s.ensureCosUploadConfig(ctx); err != nil {
			return
		}
	}
	if in.Group == "youban_publish_vip" {
		if err = s.ensureYoubanPublishVipConfig(ctx); err != nil {
			return
		}
	}
	if in.Group == "youban_publish_vip_activity" {
		if err = s.ensureYoubanPublishVipActivityConfig(ctx); err != nil {
			return
		}
	}

	res = new(sysin.GetConfigModel)
	if in.Group == "upload" {
		res.List, err = s.getUploadConfigValues(ctx)
	} else {
		res.List, err = s.getConfigValuesFromDB(ctx, in.Group)
	}
	if err != nil {
		return nil, err
	}

	// FilterMaskDemo mutates the map; never allow it to alter the cached secrets.
	res.List = simple.FilterMaskDemo(ctx, cloneConfigValues(res.List))
	return
}

// ConversionType 转换类型
func (s *sSysConfig) ConversionType(ctx context.Context, models *entity.SysConfig) (value interface{}, err error) {
	if models == nil {
		err = gerror.New("数据不存在")
		return
	}
	return consts.ConvType(models.Value, models.Type), nil
}

// UpdateConfigByGroup 更新指定分组的配置
func (s *sSysConfig) UpdateConfigByGroup(ctx context.Context, in *sysin.UpdateConfigInp) (err error) {
	if in.Group == "" {
		err = gerror.New("分组不能为空")
		return
	}
	if in.Group == "member_vip" {
		if err = s.ensureMemberVipConfig(ctx); err != nil {
			return
		}
	}
	if in.Group == "upload" {
		if err = s.ensureCosUploadConfig(ctx); err != nil {
			return
		}
	}
	if in.Group == "youban_publish_vip" {
		if err = s.ensureYoubanPublishVipConfig(ctx); err != nil {
			return
		}
	}
	if in.Group == "youban_publish_vip_activity" {
		if err = s.ensureYoubanPublishVipActivityConfig(ctx); err != nil {
			return
		}
	}
	var (
		mod    = dao.SysConfig.Ctx(ctx)
		models []*entity.SysConfig
	)

	if err = mod.Where("group", in.Group).Scan(&models); err != nil {
		return
	}

	err = dao.SysConfig.Transaction(ctx, func(ctx context.Context, tx gdb.TX) (err error) {
		if in.Group == youbanPublishVipActivityConfigGroup {
			if err = s.refreshYoubanPublishVipActivityEnabledAt(ctx, tx, in.List, models); err != nil {
				return
			}
		}
		for k, v := range in.List {
			row := s.getConfigByKey(k, models)
			// 新增
			if row == nil {
				err = gerror.Newf("暂不支持从前台添加变量，请先在数据库表[%v]中配置变量：%v", dao.SysConfig.Table(), k)
				return
			}

			// 更新
			_, err = tx.Model(dao.SysConfig.Table()).Ctx(ctx).Where("id", row.Id).Data(g.Map{"value": normalizeConfigValue(v), "updated_at": gtime.Now()}).Update()
			if err != nil {
				return
			}
		}
		if in.Group == "upload" {
			return nil
		}
		return s.syncUpdate(ctx, in)
	})

	if err != nil {
		return
	}
	if in.Group == "upload" {
		if cacheErr := s.refreshUploadConfigCache(ctx); cacheErr != nil {
			// DB is authoritative and has committed. Do not report a false save failure;
			// cache misses will fall back to DB and the cluster reload below retries it.
			g.Log().Warningf(ctx, "上传配置已保存，但刷新缓存失败: %+v", cacheErr)
		}
		if err = s.syncUpdate(ctx, in); err != nil {
			return
		}
	}

	global.PublishClusterSync(ctx, consts.ClusterSyncSysconfig, nil)
	return
}

func (s *sSysConfig) getConfigByKey(key string, models []*entity.SysConfig) *entity.SysConfig {
	if len(models) == 0 {
		return nil
	}

	for _, v := range models {
		if key == v.Key {
			return v
		}
	}
	return nil
}

func normalizeConfigValue(value interface{}) interface{} {
	switch value.(type) {
	case map[string]interface{}, []interface{}:
		return gjson.New(value).String()
	default:
		return gconv.String(value)
	}
}

func (s *sSysConfig) ensureMemberVipConfig(ctx context.Context) (err error) {
	defaults := []struct {
		key   string
		name  string
		typ   string
		value interface{}
		sort  int
	}{
		{key: "memberVipEnabled", name: "会员认证支付开关", typ: consts.ConfigTypeBool, value: true, sort: 1},
		{key: "memberVipCustomerFallback", name: "关闭支付时打开客服", typ: consts.ConfigTypeBool, value: true, sort: 2},
		{key: "memberVipDays", name: "会员认证天数", typ: consts.ConfigTypeInt, value: 30, sort: 3},
		{key: "memberVipMoney", name: "会员认证价格", typ: consts.ConfigTypeFloat64, value: 30, sort: 4},
		{key: "memberVipPayItems", name: "会员认证支付渠道", typ: consts.ConfigTypeString, value: gjson.New(defaultMemberVipConfig().PayItems).String(), sort: 5},
	}

	cols := dao.SysConfig.Columns()
	var rows []*entity.SysConfig
	if err = dao.SysConfig.Ctx(ctx).Where(cols.Group, "member_vip").Scan(&rows); err != nil {
		return
	}

	exists := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		exists[row.Key] = struct{}{}
	}

	for _, item := range defaults {
		if _, ok := exists[item.key]; ok {
			continue
		}
		_, err = dao.SysConfig.Ctx(ctx).Data(g.Map{
			cols.Group:        "member_vip",
			cols.Key:          item.key,
			cols.Name:         item.name,
			cols.Type:         item.typ,
			cols.Value:        normalizeConfigValue(item.value),
			cols.DefaultValue: normalizeConfigValue(item.value),
			cols.IsDefault:    0,
			cols.Sort:         item.sort,
			cols.Tip:          "会员认证支付配置",
			cols.Status:       consts.StatusEnabled,
			cols.CreatedAt:    gtime.Now(),
			cols.UpdatedAt:    gtime.Now(),
		}).Insert()
		if err != nil {
			return
		}
	}
	return
}

func defaultMemberVipConfig() *model.MemberVipConfig {
	return &model.MemberVipConfig{
		Enabled:          true,
		CustomerFallback: true,
		Days:             30,
		Money:            30,
		PayItems: []*model.MemberVipPayItem{
			{Label: "支付宝", PayType: consts.PayTypeRainbow, TradeType: consts.TradeTypeRainbowAliPay, Enabled: true, Money: 30},
			{Label: "微信", PayType: consts.PayTypeRainbow, TradeType: consts.TradeTypeRainbowWxPay, Enabled: true, Money: 30},
			{Label: "USDT", PayType: consts.PayTypeRainbow, TradeType: consts.TradeTypeRainbowUSDT, Enabled: true, Money: 30},
		},
	}
}

// syncUpdate 同步更新一些加载配置
func (s *sSysConfig) syncUpdate(ctx context.Context, in *sysin.UpdateConfigInp) (err error) {
	var cfg any
	switch in.Group {
	case "wechat":
		cfg, err = s.GetWechat(ctx)
		if err == nil {
			wechat.SetConfig(cfg.(*model.WechatConfig))
		}
	case "pay":
		cfg, err = s.GetPay(ctx)
		if err == nil {
			payment.SetConfig(cfg.(*model.PayConfig))
		}
	case "upload":
		cfg, err = s.GetUpload(ctx)
		if err == nil {
			storager.SetConfig(cfg.(*model.UploadConfig))
		}
	case "sms":
		cfg, err = s.GetSms(ctx)
		if err == nil {
			sms.SetConfig(cfg.(*model.SmsConfig))
		}
	}

	if err != nil {
		err = gerror.Newf("syncUpdate %v conifg fail：%+v", in.Group, err.Error())
	}
	return
}

// ClusterSync 集群同步
func (s *sSysConfig) ClusterSync(ctx context.Context, message *gredis.Message) {
	if err := s.LoadConfig(ctx); err != nil {
		g.Log().Errorf(ctx, "ClusterSync fail：%+v", err)
	}
}
