package sys

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/grand"
	"github.com/tencentyun/cos-go-sdk-v5"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/storager"
	basesysin "hotgo/internal/model/input/sysin"
)

const directUploadSessionPrefix = "youban_publish:cos_direct_upload:"

var directUploadSessionTTL = 10 * time.Minute

type mediaDirectUploadSession struct {
	TenantId     int64
	AccountId    int64
	UserId       int64
	Module       string
	Key          string
	FileName     string
	FileSize     int64
	ContentType  string
	Media        sysin.MediaUploadInp
	AttachmentId int64
	MediaId      int64
}

func (s *sSysPublish) AdminMediaDirectUploadCreate(ctx context.Context, in *sysin.MediaDirectUploadCreateInp) (*sysin.MediaDirectUploadCreateModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.createMediaDirectUpload(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) MyMediaDirectUploadCreate(ctx context.Context, in *sysin.MediaDirectUploadCreateInp) (*sysin.MediaDirectUploadCreateModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.createMediaDirectUpload(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) createMediaDirectUpload(ctx context.Context, in *sysin.MediaDirectUploadCreateInp, tenantId, accountId int64) (*sysin.MediaDirectUploadCreateModel, error) {
	if in == nil {
		return nil, gerror.New("直传参数不能为空")
	}
	if storager.GetConfig().Drive != consts.UploadDriveCos {
		return nil, gerror.New("当前存储未启用腾讯COS直传")
	}
	if err := in.MediaUploadInp.Filter(ctx); err != nil {
		return nil, err
	}
	if in.MediaType != "video" && in.MediaType != "image" {
		return nil, gerror.New("COS直传仅支持图片和视频")
	}
	if in.FileSize <= 0 {
		return nil, gerror.New("文件大小不合法")
	}
	cfg := storager.GetConfig()
	if in.MediaType == "video" && cfg.FileSize > 0 && in.FileSize > cfg.FileSize*1024*1024 {
		return nil, gerror.Newf("视频大小不能超过%vMB", cfg.FileSize)
	}
	if in.MediaType == "image" && cfg.ImageSize > 0 && in.FileSize > cfg.ImageSize*1024*1024 {
		return nil, gerror.Newf("图片大小不能超过%vMB", cfg.ImageSize)
	}
	ext := storager.Ext(in.FileName)
	if in.MediaType == "video" && !storager.IsVideoType(ext) {
		return nil, gerror.New("上传的文件不是视频")
	}
	if in.MediaType == "image" && !storager.IsImgType(ext) {
		return nil, gerror.New("上传的文件不是图片")
	}
	if _, err := s.resolveMediaEditTask(ctx, &in.MediaUploadInp, tenantId, accountId); err != nil {
		return nil, err
	}
	bucket, region, err := directUploadBucketRegionFromConfig(cfg.CosBucket, cfg.CosRegion, cfg.CosBucketURL, cfg.CosPublicURL)
	if err != nil {
		return nil, err
	}
	key := storager.GenFullPath(storager.GetConfig().CosPath, "."+ext)
	sessionId := grand.S(40)
	session := &mediaDirectUploadSession{TenantId: tenantId, AccountId: accountId, UserId: contexts.GetUserId(ctx), Module: contexts.GetModule(ctx), Key: key, FileName: in.FileName, FileSize: in.FileSize, ContentType: in.ContentType, Media: in.MediaUploadInp}
	if err = cache.Instance().Set(ctx, directUploadSessionPrefix+sessionId, session, directUploadSessionTTL); err != nil {
		return nil, gerror.Wrap(err, "创建直传会话失败")
	}
	return &sysin.MediaDirectUploadCreateModel{
		SessionId:    sessionId,
		Bucket:       bucket,
		Region:       region,
		Key:          key,
		UploadDomain: directUploadDomain(cfg.CosUploadURL),
	}, nil
}

func (s *sSysPublish) AdminMediaDirectUploadSign(ctx context.Context, in *sysin.MediaDirectUploadSignInp) (*sysin.MediaDirectUploadSignModel, error) {
	return s.signMediaDirectUpload(ctx, in, true)
}
func (s *sSysPublish) MyMediaDirectUploadSign(ctx context.Context, in *sysin.MediaDirectUploadSignInp) (*sysin.MediaDirectUploadSignModel, error) {
	return s.signMediaDirectUpload(ctx, in, false)
}

func (s *sSysPublish) signMediaDirectUpload(ctx context.Context, in *sysin.MediaDirectUploadSignInp, admin bool) (*sysin.MediaDirectUploadSignModel, error) {
	if in == nil {
		return nil, gerror.New("签名参数不能为空")
	}
	session, err := loadMediaDirectUploadSession(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	if err = verifyDirectUploadIdentity(ctx, session, admin); err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method != http.MethodPut && method != http.MethodPost && method != http.MethodGet && method != http.MethodDelete && method != http.MethodHead {
		return nil, gerror.New("不允许签发该COS请求")
	}
	requestKey := strings.TrimPrefix(strings.TrimSpace(in.Key), "/")
	bucketMultipartQuery := requestKey == "" && method == http.MethodGet && directUploadQueryValue(in.Query, "prefix") == session.Key && directUploadQueryHasKey(in.Query, "uploads")
	if requestKey == "" && !bucketMultipartQuery {
		return nil, gerror.New("对象路径不能为空")
	}
	if requestKey != "" && requestKey != session.Key {
		return nil, gerror.New("对象路径与上传会话不匹配")
	}
	query := url.Values{}
	for key, values := range in.Query {
		if key != "uploads" && key != "uploadId" && key != "partNumber" && key != "prefix" {
			continue
		}
		for _, value := range values {
			query.Add(key, value)
		}
	}
	headers := http.Header{}
	for key, values := range in.Headers {
		lower := strings.ToLower(key)
		if lower != "content-type" && lower != "content-length" && !strings.HasPrefix(lower, "x-cos-") {
			continue
		}
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	client, err := storager.NewCosClient()
	if err != nil {
		return nil, err
	}
	sign := client.Object.GetSignature(ctx, method, requestKey, storager.GetConfig().CosSecretId, storager.GetConfig().CosSecretKey, 5*time.Minute, &cos.PresignedURLOptions{Query: &query, Header: &headers})
	if sign == "" {
		return nil, gerror.New("生成COS上传签名失败")
	}
	return &sysin.MediaDirectUploadSignModel{Authorization: sign}, nil
}

func directUploadQueryValue(query map[string][]string, key string) string {
	for queryKey, values := range query {
		if strings.EqualFold(queryKey, key) && len(values) > 0 {
			return strings.TrimPrefix(strings.TrimSpace(values[0]), "/")
		}
	}
	return ""
}

func directUploadQueryHasKey(query map[string][]string, key string) bool {
	for queryKey := range query {
		if strings.EqualFold(queryKey, key) {
			return true
		}
	}
	return false
}

func (s *sSysPublish) AdminMediaDirectUploadComplete(ctx context.Context, in *sysin.MediaDirectUploadCompleteInp, poster *ghttp.UploadFile) (*sysin.MediaModel, error) {
	return s.completeMediaDirectUpload(ctx, in, poster, true)
}
func (s *sSysPublish) MyMediaDirectUploadComplete(ctx context.Context, in *sysin.MediaDirectUploadCompleteInp, poster *ghttp.UploadFile) (*sysin.MediaModel, error) {
	return s.completeMediaDirectUpload(ctx, in, poster, false)
}

func (s *sSysPublish) completeMediaDirectUpload(ctx context.Context, in *sysin.MediaDirectUploadCompleteInp, poster *ghttp.UploadFile, admin bool) (*sysin.MediaModel, error) {
	if in == nil {
		return nil, gerror.New("完成参数不能为空")
	}
	session, err := loadMediaDirectUploadSession(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	if err = verifyDirectUploadIdentity(ctx, session, admin); err != nil {
		return nil, err
	}
	if session.MediaId > 0 {
		return s.mediaViewById(ctx, session.MediaId)
	}
	client, err := storager.NewCosClient()
	if err != nil {
		return nil, err
	}
	head, err := client.Object.Head(ctx, session.Key, nil)
	if err != nil {
		return nil, gerror.Wrap(err, "校验COS直传文件失败")
	}
	if head.ContentLength != session.FileSize {
		return nil, gerror.Newf("COS文件大小不匹配，预期:%d 实际:%d", session.FileSize, head.ContentLength)
	}
	attachment := new(basesysin.AttachmentListModel)
	if session.AttachmentId > 0 {
		if err = dao.SysAttachment.Ctx(ctx).WherePri(session.AttachmentId).Scan(&attachment.SysAttachment); err != nil {
			return nil, gerror.Wrap(err, "读取直传附件失败")
		}
	}
	if attachment.Id <= 0 {
		stored, writeErr := storager.WriteDirectUploadAttachment(ctx, session.FileName, session.FileSize, session.Key)
		if writeErr != nil {
			return nil, gerror.Wrap(writeErr, "保存直传附件失败")
		}
		attachment.SysAttachment = *stored
		session.AttachmentId = stored.Id
		if err = cache.Instance().Set(ctx, directUploadSessionPrefix+in.SessionId, session, directUploadSessionTTL); err != nil {
			return nil, err
		}
	}
	task, err := s.resolveMediaEditTask(ctx, &session.Media, session.TenantId, session.AccountId)
	if err != nil {
		return nil, err
	}
	var posterAttachment *basesysin.AttachmentListModel
	if poster != nil {
		posterAttachment, err = uploadMediaPoster(ctx, poster)
		if err != nil {
			return nil, err
		}
	}
	media, err := s.saveMediaAttachment(ctx, task, &session.Media, attachment, posterAttachment, nil, "")
	if err != nil {
		return nil, err
	}
	session.MediaId = media.Id
	_ = cache.Instance().Set(ctx, directUploadSessionPrefix+in.SessionId, session, directUploadSessionTTL)
	if err = s.enqueueMediaProcess(ctx, media.Id, 0); err != nil {
		g.Log().Warningf(ctx, "直传媒体处理任务入队失败 media_id:%d err:%+v", media.Id, err)
	}
	return media, nil
}

func loadMediaDirectUploadSession(ctx context.Context, sessionId string) (*mediaDirectUploadSession, error) {
	value, err := cache.Instance().Get(ctx, directUploadSessionPrefix+strings.TrimSpace(sessionId))
	if err != nil {
		return nil, gerror.Wrap(err, "读取直传会话失败")
	}
	if value.IsNil() {
		return nil, gerror.New("直传会话已过期，请重新上传")
	}
	session := new(mediaDirectUploadSession)
	if err = value.Struct(session); err != nil {
		return nil, gerror.Wrap(err, "解析直传会话失败")
	}
	return session, nil
}

func verifyDirectUploadIdentity(ctx context.Context, session *mediaDirectUploadSession, admin bool) error {
	if session == nil || session.UserId != contexts.GetUserId(ctx) || session.Module != contexts.GetModule(ctx) {
		return gerror.New("无权使用该直传会话")
	}
	if admin && session.AccountId != 0 {
		return gerror.New("直传会话类型不匹配")
	}
	if !admin && session.AccountId <= 0 {
		return gerror.New("直传会话类型不匹配")
	}
	return nil
}

func directUploadBucketRegion(rawURL string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Hostname() == "" {
		return "", "", gerror.New("COS Bucket访问域名配置不正确")
	}
	parts := strings.Split(u.Hostname(), ".")
	if len(parts) < 4 || parts[1] != "cos" {
		return "", "", gerror.New("无法从COS域名识别Bucket和Region")
	}
	return parts[0], parts[2], nil
}

func directUploadBucketRegionFromConfig(bucket, region, bucketURL, publicURL string) (string, string, error) {
	bucket = strings.TrimSpace(bucket)
	region = strings.TrimSpace(region)
	if bucket != "" || region != "" {
		if bucket == "" || region == "" {
			return "", "", gerror.New("COS Bucket和Region必须同时配置")
		}
		return bucket, region, nil
	}
	for _, candidate := range []string{bucketURL, publicURL} {
		parsedBucket, parsedRegion, err := directUploadBucketRegion(candidate)
		if err == nil {
			return parsedBucket, parsedRegion, nil
		}
	}
	return "", "", gerror.New("无法识别COS Bucket和Region，请在后台明确配置COS Bucket与Region，或配置 *.cos.<region>.myqcloud.com 源站域名")
}

func directUploadDomain(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return ""
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
}

func (s *sSysPublish) mediaViewById(ctx context.Context, id int64) (*sysin.MediaModel, error) {
	res := new(sysin.MediaModel)
	if err := pdao.YoubanPublishMedia.Ctx(ctx).WherePri(id).Scan(res); err != nil {
		return nil, err
	}
	if res.Id <= 0 {
		return nil, gerror.New("直传媒体不存在")
	}
	return res, nil
}
