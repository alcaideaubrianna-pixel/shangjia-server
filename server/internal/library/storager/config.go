// Package storager
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package storager

import (
	"context"
	"hotgo/internal/dao"
	"hotgo/internal/model"
	"net/url"
	"os"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

var config *model.UploadConfig

// ApplyUploadEndpointEnv keeps server-side OSS transfers configurable per role.
// Public media URLs remain in OssBucketURL; browser signatures use the public
// counterpart of this endpoint through OssPublicEndpoint.
func ApplyUploadEndpointEnv(c *model.UploadConfig) (*model.UploadConfig, error) {
	endpoint := strings.TrimSpace(os.Getenv("YOUBAN_STORAGE_OSS_ENDPOINT"))
	if c == nil || endpoint == "" {
		return c, nil
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") || u.Hostname() == "" {
		return nil, gerror.New("YOUBAN_STORAGE_OSS_ENDPOINT 格式不正确")
	}
	// The OSS SDK adds the bucket name itself. Accept the bucket endpoint
	// supplied by operators without producing bucket.bucket.endpoint URLs.
	u.Host = strings.TrimPrefix(strings.ToLower(u.Host), strings.ToLower(strings.TrimSpace(c.OssBucket))+".")
	u.Path = ""
	copy := *c
	copy.OssEndpoint = u.String()
	return &copy, nil
}

func SetConfig(c *model.UploadConfig) {
	config = c
}

func GetConfig() *model.UploadConfig {
	return config
}

func GetModel(ctx context.Context) *gdb.Model {
	return dao.SysAttachment.Ctx(ctx)
}
