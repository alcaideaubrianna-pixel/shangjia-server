// Package location
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package location

import (
	"context"
	"fmt"
	"hotgo/internal/consts"
	"hotgo/utility/validate"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/encoding/gcharset"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/text/gstr"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/kayon/iploc"
)

const (
	whoisApi   = "https://whois.pconline.com.cn/ipJson.jsp?json=true&ip="
	ipWhoisApi = "https://ipwho.is/"
	ipInfoApi  = "https://ipinfo.io/"
	dyndns     = "http://members.3322.org/dyndns/getip" // 备用："https://ifconfig.co/ip"

	ipLocationCacheTable = "hg_sys_ip_location_cache"
	ipLocationCacheTTL   = 30 * 24 * time.Hour
)

type IpLocationData struct {
	Ip           string `json:"ip"`
	Country      string `json:"country"`
	Region       string `json:"region"`
	Province     string `json:"province"`
	ProvinceCode int64  `json:"province_code"`
	City         string `json:"city"`
	CityCode     int64  `json:"city_code"`
	Area         string `json:"area"`
	AreaCode     int64  `json:"area_code"`
}

type WhoisRegionData struct {
	Ip         string `json:"ip"`
	Pro        string `json:"pro" `
	ProCode    string `json:"proCode" `
	City       string `json:"city" `
	CityCode   string `json:"cityCode"`
	Region     string `json:"region"`
	RegionCode string `json:"regionCode"`
	Addr       string `json:"addr"`
	Err        string `json:"err"`
}

type IpInfoRegionData struct {
	Ip      string `json:"ip"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Org     string `json:"org"`
}

type IpWhoisRegionData struct {
	Ip            string `json:"ip"`
	Success       bool   `json:"success"`
	Type          string `json:"type"`
	Continent     string `json:"continent"`
	ContinentCode string `json:"continent_code"`
	Country       string `json:"country"`
	CountryCode   string `json:"country_code"`
	Region        string `json:"region"`
	RegionCode    string `json:"region_code"`
	City          string `json:"city"`
	Message       string `json:"message"`
	Connection    struct {
		ASN    int    `json:"asn"`
		Org    string `json:"org"`
		ISP    string `json:"isp"`
		Domain string `json:"domain"`
	} `json:"connection"`
}

var (
	defaultRetry                 int64 = 3 // 默认重试次数
	initIpLocationCacheTableOnce sync.Once
	initIpLocationCacheTableErr  error
)

type ipLocationCacheRow struct {
	Ip           string      `json:"ip" orm:"ip"`
	Country      string      `json:"country" orm:"country"`
	Region       string      `json:"region" orm:"region"`
	Province     string      `json:"province" orm:"province"`
	ProvinceCode int64       `json:"provinceCode" orm:"province_code"`
	City         string      `json:"city" orm:"city"`
	CityCode     int64       `json:"cityCode" orm:"city_code"`
	Area         string      `json:"area" orm:"area"`
	AreaCode     int64       `json:"areaCode" orm:"area_code"`
	ExpiresAt    *gtime.Time `json:"expiresAt" orm:"expires_at"`
}

// WhoisLocation 通过Whois接口查询IP归属地
func WhoisLocation(ctx context.Context, ip string, retry ...int64) (*IpLocationData, error) {
	response, err := g.Client().Timeout(10*time.Second).Get(ctx, whoisApi+ip)
	if err != nil {
		return nil, err
	}

	defer response.Close()

	str, err := gcharset.ToUTF8("GBK", response.ReadAllString())
	if err != nil {
		return nil, err
	}

	// 利用重试机制缓解高并发情况下限流的影响
	// 毕竟这是一个免费的接口，如果你对IP归属地定位要求毕竟高，可以考虑换个付费接口
	if response.StatusCode != http.StatusOK {
		retryCount := defaultRetry
		if len(retry) > 0 {
			retryCount = retry[0]
		}
		if retryCount > 0 {
			retryCount--
			return WhoisLocation(ctx, ip, retryCount)
		}
	}

	var who *WhoisRegionData
	if err = gconv.Scan([]byte(str), &who); err != nil {
		err = gerror.Newf("WhoisLocation Scan err:%v, str:%v", err, str)
		return nil, err
	}
	return &IpLocationData{
		Ip:           who.Ip,
		Region:       who.Addr,
		Province:     who.Pro,
		ProvinceCode: gconv.Int64(who.ProCode),
		City:         who.City,
		CityCode:     gconv.Int64(who.CityCode),
		Area:         who.Region,
		AreaCode:     gconv.Int64(who.RegionCode),
	}, nil
}

// IpInfoLocation 通过全球 IP 库查询 IP 归属地，主要用于 whois 对港澳台/海外 IP 返回不准确时兜底。
func IpInfoLocation(ctx context.Context, ip string) (*IpLocationData, error) {
	response, err := g.Client().Timeout(5*time.Second).Get(ctx, ipInfoApi+ip+"/json")
	if err != nil {
		return nil, err
	}
	defer response.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipinfo status:%d", response.StatusCode)
	}

	var info *IpInfoRegionData
	if err = gconv.Scan([]byte(response.ReadAllString()), &info); err != nil {
		return nil, err
	}
	if info == nil {
		return nil, fmt.Errorf("ipinfo empty response")
	}

	province := normalizeIpInfoProvince(info.Country, info.Region)
	return &IpLocationData{
		Ip:       info.Ip,
		Country:  info.Country,
		Region:   info.Region,
		Province: province,
		City:     info.City,
		Area:     info.Org,
	}, nil
}

// IpWhoisLocation 通过 ipwho.is 查询全球 IP 归属地。该接口免费、无需 token，业务侧必须配合缓存使用。
func IpWhoisLocation(ctx context.Context, ip string) (*IpLocationData, error) {
	response, err := g.Client().Timeout(2*time.Second).Get(ctx, ipWhoisApi+ip)
	if err != nil {
		return nil, err
	}
	defer response.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipwhois status:%d", response.StatusCode)
	}

	var info *IpWhoisRegionData
	if err = gconv.Scan([]byte(response.ReadAllString()), &info); err != nil {
		return nil, err
	}
	if info == nil {
		return nil, fmt.Errorf("ipwhois empty response")
	}
	if !info.Success {
		return nil, fmt.Errorf("ipwhois failed: %s", info.Message)
	}

	province := normalizeGlobalProvince(info.CountryCode, info.Region)
	return &IpLocationData{
		Ip:       info.Ip,
		Country:  info.CountryCode,
		Region:   info.Region,
		Province: province,
		City:     info.City,
		Area:     strings.TrimSpace(info.Connection.Org),
	}, nil
}

func normalizeIpInfoProvince(country string, region string) string {
	return normalizeGlobalProvince(country, region)
}

func normalizeGlobalProvince(country string, region string) string {
	switch strings.ToUpper(strings.TrimSpace(country)) {
	case "HK":
		return "香港"
	case "MO":
		return "澳门"
	case "TW":
		return "台湾"
	case "CN":
		return strings.TrimSpace(region)
	default:
		return strings.TrimSpace(region)
	}
}

func needsGlobalLocationFallback(data *IpLocationData) bool {
	if data == nil {
		return true
	}
	return strings.TrimSpace(data.Province) == ""
}

func getCachedLocationFromDB(ctx context.Context, ip string) (*IpLocationData, error) {
	if err := ensureIpLocationCacheTable(ctx); err != nil {
		return nil, err
	}
	var row *ipLocationCacheRow
	err := g.DB().Model(ipLocationCacheTable).
		Ctx(ctx).
		Where("ip", ip).
		Where("expires_at > ?", gtime.Now()).
		Scan(&row)
	if err != nil || row == nil {
		return nil, err
	}
	return &IpLocationData{
		Ip:           row.Ip,
		Country:      row.Country,
		Region:       row.Region,
		Province:     row.Province,
		ProvinceCode: row.ProvinceCode,
		City:         row.City,
		CityCode:     row.CityCode,
		Area:         row.Area,
		AreaCode:     row.AreaCode,
	}, nil
}

func setCachedLocationToDB(ctx context.Context, ip string, data *IpLocationData) {
	if data == nil {
		return
	}
	if err := ensureIpLocationCacheTable(ctx); err != nil {
		g.Log().Warningf(ctx, "初始化IP归属地缓存表失败 ip:%s err:%+v", ip, err)
		return
	}
	expiresAt := gtime.New(time.Now().Add(ipLocationCacheTTL))
	now := gtime.Now()
	if isPgsql() {
		_, err := g.DB().Exec(ctx, `
INSERT INTO hg_sys_ip_location_cache
  (ip, country, region, province, province_code, city, city_code, area, area_code, expires_at, created_at, updated_at)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (ip) DO UPDATE SET
  country = EXCLUDED.country,
  region = EXCLUDED.region,
  province = EXCLUDED.province,
  province_code = EXCLUDED.province_code,
  city = EXCLUDED.city,
  city_code = EXCLUDED.city_code,
  area = EXCLUDED.area,
  area_code = EXCLUDED.area_code,
  expires_at = EXCLUDED.expires_at,
  updated_at = EXCLUDED.updated_at`,
			ip, data.Country, data.Region, data.Province, data.ProvinceCode, data.City, data.CityCode, data.Area, data.AreaCode, expiresAt, now, now,
		)
		if err != nil {
			g.Log().Warningf(ctx, "写入IP归属地缓存失败 ip:%s err:%+v", ip, err)
		}
		return
	}

	_, err := g.DB().Exec(ctx, `
INSERT INTO hg_sys_ip_location_cache
  (ip, country, region, province, province_code, city, city_code, area, area_code, expires_at, created_at, updated_at)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  country = VALUES(country),
  region = VALUES(region),
  province = VALUES(province),
  province_code = VALUES(province_code),
  city = VALUES(city),
  city_code = VALUES(city_code),
  area = VALUES(area),
  area_code = VALUES(area_code),
  expires_at = VALUES(expires_at),
  updated_at = VALUES(updated_at)`,
		ip, data.Country, data.Region, data.Province, data.ProvinceCode, data.City, data.CityCode, data.Area, data.AreaCode, expiresAt, now, now,
	)
	if err != nil {
		g.Log().Warningf(ctx, "写入IP归属地缓存失败 ip:%s err:%+v", ip, err)
	}
}

func ensureIpLocationCacheTable(ctx context.Context) error {
	initIpLocationCacheTableOnce.Do(func() {
		defer func() {
			if exception := recover(); exception != nil {
				initIpLocationCacheTableErr = fmt.Errorf("init ip location cache table panic: %v", exception)
			}
		}()
		var sql string
		if isPgsql() {
			sql = `
CREATE TABLE IF NOT EXISTS hg_sys_ip_location_cache (
  ip varchar(64) PRIMARY KEY,
  country varchar(64),
  region varchar(128),
  province varchar(128),
  province_code bigint NOT NULL DEFAULT 0,
  city varchar(128),
  city_code bigint NOT NULL DEFAULT 0,
  area varchar(255),
  area_code bigint NOT NULL DEFAULT 0,
  expires_at timestamp NOT NULL,
  created_at timestamp,
  updated_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_sys_ip_location_cache_expires ON hg_sys_ip_location_cache (expires_at);`
		} else {
			sql = `
CREATE TABLE IF NOT EXISTS hg_sys_ip_location_cache (
  ip varchar(64) NOT NULL,
  country varchar(64) DEFAULT NULL,
  region varchar(128) DEFAULT NULL,
  province varchar(128) DEFAULT NULL,
  province_code bigint NOT NULL DEFAULT 0,
  city varchar(128) DEFAULT NULL,
  city_code bigint NOT NULL DEFAULT 0,
  area varchar(255) DEFAULT NULL,
  area_code bigint NOT NULL DEFAULT 0,
  expires_at datetime NOT NULL,
  created_at datetime DEFAULT NULL,
  updated_at datetime DEFAULT NULL,
  PRIMARY KEY (ip),
  KEY idx_sys_ip_location_cache_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`
		}
		for _, statement := range strings.Split(sql, ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err := g.DB().Exec(ctx, statement); err != nil {
				initIpLocationCacheTableErr = err
				return
			}
		}
	})
	return initIpLocationCacheTableErr
}

func isPgsql() bool {
	defer func() {
		recover()
	}()
	return strings.EqualFold(g.DB().GetConfig().Type, consts.DBPgsql)
}

// Cz88Find 通过Cz88的IP库查询IP归属地
func Cz88Find(ctx context.Context, ip string) (*IpLocationData, error) {
	loc, err := iploc.OpenWithoutIndexes("./resource/ip/qqwry-utf8.dat")
	if err != nil {
		return nil, fmt.Errorf("%v for help, please go to: https://github.com/kayon/iploc", err.Error())
	}

	detail := loc.Find(ip)
	if detail == nil {
		return nil, fmt.Errorf("no ip data is queried. procedure:%v", ip)
	}
	return &IpLocationData{
		Ip:       ip,
		Country:  detail.Country,
		Region:   detail.Region,
		Province: detail.Province,
		City:     detail.City,
		Area:     detail.County,
	}, nil
}

// GetLocation 获取IP归属地信息
func GetLocation(ctx context.Context, ip string) (data *IpLocationData, err error) {
	if !validate.IsIp(ip) {
		return nil, fmt.Errorf("invalid input ip:%v", ip)
	}

	if validate.IsLocalIPAddr(ip) {
		return // nil, fmt.Errorf("must be a public ip:%v", ip)
	}

	if cache.Contains(ip) {
		return cache.GetIpCache(ip)
	}

	if data, err = getCachedLocationFromDB(ctx, ip); err == nil && data != nil {
		cache.SetIpCache(ip, data)
		return data, nil
	}

	cache.Lock()
	defer cache.Unlock()

	if cache.Contains(ip) {
		return cache.GetIpCache(ip)
	}
	if data, err = getCachedLocationFromDB(ctx, ip); err == nil && data != nil {
		cache.SetIpCache(ip, data)
		return data, nil
	}

	mode := g.Cfg().MustGet(ctx, "system.ipMethod", "cz88").String()
	switch mode {
	case "ipwhois":
		data, err = IpWhoisLocation(ctx, ip)
	case "whois":
		data, err = WhoisLocation(ctx, ip)
		if err == nil && needsGlobalLocationFallback(data) {
			if fallback, fallbackErr := IpInfoLocation(ctx, ip); fallbackErr == nil && fallback != nil && strings.TrimSpace(fallback.Province) != "" {
				data = fallback
			}
		}
	default:
		data, err = Cz88Find(ctx, ip)
	}

	if err == nil && data != nil {
		cache.SetIpCache(ip, data)
		setCachedLocationToDB(ctx, ip, data)
	}
	return
}

// GetPublicIP 获取公网IP
func GetPublicIP(ctx context.Context) (ip string, err error) {
	var data *WhoisRegionData
	err = g.Client().Timeout(10*time.Second).GetVar(ctx, whoisApi).Scan(&data)
	if err != nil {
		g.Log().Info(ctx, "GetPublicIP fail, alternatives are being tried.")
		return GetPublicIP2()
	}

	if data == nil {
		g.Log().Info(ctx, "publicIP address Parsing failure, check the network and firewall blocking.")
		return "0.0.0.0", nil
	}
	return data.Ip, nil
}

func GetPublicIP2() (ip string, err error) {
	response, err := http.Get(dyndns)
	if err != nil {
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return
	}
	ip = strings.ReplaceAll(string(body), "\n", "")
	return
}

// GetLocalIP 获取服务器内网IP
func GetLocalIP() (ip string, err error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}
	for _, addr := range addrs {
		ipAddr, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ipAddr.IP.IsLoopback() {
			continue
		}
		if !ipAddr.IP.IsGlobalUnicast() {
			continue
		}
		return ipAddr.IP.String(), nil
	}
	return
}

// GetClientIp 获取客户端IP
func GetClientIp(r *ghttp.Request) string {
	if r == nil {
		return ""
	}
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.GetClientIp()
	}

	// 兼容部分云厂商CDN，如果存在多个，默认取第一个
	if gstr.Contains(ip, ",") {
		ip = gstr.StrTillEx(ip, ",")
	}

	if gstr.Contains(ip, ", ") {
		ip = gstr.StrTillEx(ip, ", ")
	}
	return ip
}
