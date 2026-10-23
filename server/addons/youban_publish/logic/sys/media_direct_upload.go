package sys

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
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
	"hotgo/internal/model"
	basesysin "hotgo/internal/model/input/sysin"
	isc "hotgo/internal/service"
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
	Provider     string
	Bucket       string
	Region       string
	ConfigDigest string
	FileMD5      string
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
	startedAt := time.Now()
	defer func() {
		g.Log().Infof(ctx, "发布媒体直传 create 完成 tenantId:%d accountId:%d profileId:%d mediaType:%s size:%d durationMs:%d", tenantId, accountId, func() int64 {
			if in == nil {
				return 0
			}
			return in.ProfileId
		}(), func() string {
			if in == nil {
				return ""
			}
			return in.MediaType
		}(), func() int64 {
			if in == nil {
				return 0
			}
			return in.FileSize
		}(), time.Since(startedAt).Milliseconds())
	}()
	if in == nil {
		return nil, gerror.New("直传参数不能为空")
	}
	cfg, err := loadDirectUploadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Drive != consts.UploadDriveCos && cfg.Drive != consts.UploadDriveOss && cfg.Drive != consts.UploadDriveR2 {
		return nil, gerror.New("当前存储不支持媒体直传")
	}
	if err := in.MediaUploadInp.Filter(ctx); err != nil {
		return nil, err
	}
	if in.MediaType != "video" && in.MediaType != "image" {
		return nil, gerror.New("媒体直传仅支持图片和视频")
	}
	if in.FileSize <= 0 {
		return nil, gerror.New("文件大小不合法")
	}
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
	bucket, region, basePath := cfg.CosBucket, cfg.CosRegion, cfg.CosPath
	if cfg.Drive == consts.UploadDriveOss {
		bucket, basePath = cfg.OssBucket, cfg.OssPath
		region = "oss"
	} else if cfg.Drive == consts.UploadDriveR2 {
		bucket, region = cfg.R2Bucket, cfg.R2Region
		if region == "" {
			region = "auto"
		}
	} else {
		bucket, region, err = directUploadBucketRegionFromConfig(cfg.CosBucket, cfg.CosRegion, cfg.CosBucketURL, cfg.CosPublicURL)
		if err != nil {
			return nil, err
		}
	}
	sessionId := grand.S(40)
	key, err := storager.GenStagingPath(basePath, sessionId, ext)
	if err != nil {
		return nil, err
	}
	session := &mediaDirectUploadSession{
		TenantId: tenantId, AccountId: accountId, UserId: contexts.GetUserId(ctx), Module: contexts.GetModule(ctx),
		Key: key, FileName: in.FileName, FileSize: in.FileSize, ContentType: in.ContentType, Provider: cfg.Drive,
		Bucket: bucket, Region: region, ConfigDigest: directUploadConfigDigest(cfg), Media: in.MediaUploadInp,
	}
	if err = cache.Instance().Set(ctx, directUploadSessionPrefix+sessionId, session, directUploadSessionTTL); err != nil {
		return nil, gerror.Wrap(err, "创建直传会话失败")
	}
	res := &sysin.MediaDirectUploadCreateModel{
		Provider:  cfg.Drive,
		SessionId: sessionId,
		Bucket:    bucket,
		Region:    region,
		Key:       key,
	}
	if cfg.Drive == consts.UploadDriveOss {
		uploadURL, signErr := storager.OssPresignedPutObject(ctx, key, in.ContentType, 10*time.Minute)
		if signErr != nil {
			return nil, signErr
		}
		res.UploadURL = uploadURL
	} else if cfg.Drive == consts.UploadDriveR2 {
		uploadURL, signErr := storager.R2PresignedPutObject(ctx, key, 10*time.Minute)
		if signErr != nil {
			return nil, signErr
		}
		res.UploadURL = uploadURL.String()
	}
	return res, nil
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
	cfg, err := validateDirectUploadConfig(ctx, session)
	if err != nil {
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
	client, err := storager.NewCosClientWithConfig(cfg)
	if err != nil {
		return nil, err
	}
	sign := client.Object.GetSignature(ctx, method, requestKey, cfg.CosSecretId, cfg.CosSecretKey, 5*time.Minute, &cos.PresignedURLOptions{Query: &query, Header: &headers})
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
	startedAt := time.Now()
	stageStartedAt := startedAt
	logStage := func(stage string) {
		g.Log().Infof(ctx, "发布媒体直传 complete 阶段 stage:%s durationMs:%d totalMs:%d", stage, time.Since(stageStartedAt).Milliseconds(), time.Since(startedAt).Milliseconds())
		stageStartedAt = time.Now()
	}
	defer func() {
		g.Log().Infof(ctx, "发布媒体直传 complete 完成 durationMs:%d", time.Since(startedAt).Milliseconds())
	}()
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
	cfg, err := validateDirectUploadConfig(ctx, session)
	if err != nil {
		return nil, err
	}
	logStage("session")
	if session.MediaId > 0 {
		return s.mediaViewById(ctx, session.MediaId)
	}
	if cfg.Drive == consts.UploadDriveOss {
		head, headErr := storager.OssHeadObject(ctx, session.Key)
		if headErr != nil {
			return nil, gerror.Wrap(headErr, "校验OSS直传文件失败")
		}
		if head.Size != session.FileSize {
			return nil, gerror.Newf("OSS文件大小不匹配，预期:%d 实际:%d", session.FileSize, head.Size)
		}
		session.FileMD5, err = directUploadOssMD5(ctx, session.Key)
		if err != nil {
			return nil, err
		}
		if err = cache.Instance().Set(ctx, directUploadSessionPrefix+in.SessionId, session, directUploadSessionTTL); err != nil {
			return nil, err
		}
	} else if cfg.Drive == consts.UploadDriveR2 {
		head, headErr := storager.R2HeadObject(ctx, session.Key)
		if headErr != nil {
			return nil, gerror.Wrap(headErr, "校验R2直传文件失败")
		}
		if head.Size != session.FileSize {
			return nil, gerror.Newf("R2文件大小不匹配，预期:%d 实际:%d", session.FileSize, head.Size)
		}
		session.FileMD5, err = directUploadR2MD5(ctx, session.Key)
		if err != nil {
			return nil, err
		}
		if err = cache.Instance().Set(ctx, directUploadSessionPrefix+in.SessionId, session, directUploadSessionTTL); err != nil {
			return nil, err
		}
	} else {
		client, clientErr := storager.NewCosClientWithConfig(cfg)
		if clientErr != nil {
			return nil, clientErr
		}
		head, headErr := client.Object.Head(ctx, session.Key, nil)
		if headErr != nil {
			return nil, gerror.Wrap(headErr, "校验COS直传文件失败")
		}
		if head.Body != nil {
			_ = head.Body.Close()
		}
		if head.ContentLength != session.FileSize {
			return nil, gerror.Newf("COS文件大小不匹配，预期:%d 实际:%d", session.FileSize, head.ContentLength)
		}
		if session.FileMD5 == "" {
			session.FileMD5, err = directUploadObjectMD5(ctx, client, session.Key)
			if err != nil {
				return nil, err
			}
		}
	}
	logStage("object_head")
	if cfg.Drive == consts.UploadDriveOss {
		finalKey, pathErr := storager.GenContentPath(cfg.OssPath, session.FileMD5, storager.Ext(session.FileName))
		if pathErr != nil {
			return nil, pathErr
		}
		if finalKey != session.Key {
			if err = storager.OssCopyObject(ctx, session.Key, finalKey); err != nil {
				return nil, gerror.Wrap(err, "归档OSS直传文件失败")
			}
			stagingKey := session.Key
			session.Key = finalKey
			if err = cache.Instance().Set(ctx, directUploadSessionPrefix+in.SessionId, session, directUploadSessionTTL); err != nil {
				return nil, err
			}
			if deleteErr := storager.OssDeleteObject(ctx, stagingKey); deleteErr != nil {
				g.Log().Warningf(ctx, "删除OSS直传临时对象失败 key:%s err:%+v", stagingKey, deleteErr)
			}
		}
	} else if cfg.Drive == consts.UploadDriveR2 {
		finalKey, pathErr := storager.GenContentPath(cfg.CosPath, session.FileMD5, storager.Ext(session.FileName))
		if pathErr != nil {
			return nil, pathErr
		}
		if finalKey != session.Key {
			if err = storager.R2CopyObject(ctx, session.Key, finalKey); err != nil {
				return nil, gerror.Wrap(err, "归档R2直传文件失败")
			}
			stagingKey := session.Key
			session.Key = finalKey
			if err = cache.Instance().Set(ctx, directUploadSessionPrefix+in.SessionId, session, directUploadSessionTTL); err != nil {
				return nil, err
			}
			if deleteErr := storager.R2DeleteObject(ctx, stagingKey); deleteErr != nil {
				g.Log().Warningf(ctx, "删除R2直传临时对象失败 key:%s err:%+v", stagingKey, deleteErr)
			}
		}
	} else {
		client, clientErr := storager.NewCosClientWithConfig(cfg)
		if clientErr != nil {
			return nil, clientErr
		}
		finalKey, pathErr := storager.GenContentPath(cfg.CosPath, session.FileMD5, storager.Ext(session.FileName))
		if pathErr != nil {
			return nil, pathErr
		}
		if finalKey != session.Key {
			if err = promoteDirectUploadObject(ctx, client, cfg, session.Key, finalKey, session.FileSize); err != nil {
				return nil, err
			}
			session.Key = finalKey
			_ = cache.Instance().Set(ctx, directUploadSessionPrefix+in.SessionId, session, directUploadSessionTTL)
		}
	}
	logStage("content_promote")
	attachment := new(basesysin.AttachmentListModel)
	if session.AttachmentId > 0 {
		if err = dao.SysAttachment.Ctx(ctx).WherePri(session.AttachmentId).Scan(&attachment.SysAttachment); err != nil {
			return nil, gerror.Wrap(err, "读取直传附件失败")
		}
	}
	if attachment.Id <= 0 {
		stored, writeErr := storager.WriteDirectUploadAttachment(ctx, session.FileName, session.FileSize, session.FileMD5, session.Key)
		if writeErr != nil {
			return nil, gerror.Wrap(writeErr, "保存直传附件失败")
		}
		attachment.SysAttachment = *stored
		session.AttachmentId = stored.Id
		if err = cache.Instance().Set(ctx, directUploadSessionPrefix+in.SessionId, session, directUploadSessionTTL); err != nil {
			return nil, err
		}
	}
	logStage("attachment")
	task, err := s.resolveMediaEditTask(ctx, &session.Media, session.TenantId, session.AccountId)
	if err != nil {
		return nil, err
	}
	// Poster generation/upload is intentionally deferred to the media worker.
	// The parameter remains accepted so older clients can complete sessions.
	_ = poster
	media, err := s.saveMediaAttachment(ctx, task, &session.Media, attachment, nil, nil, "", false)
	if err != nil {
		return nil, err
	}
	logStage("media_save")
	session.MediaId = media.Id
	_ = cache.Instance().Set(ctx, directUploadSessionPrefix+in.SessionId, session, directUploadSessionTTL)
	if err = s.enqueueMediaProcess(ctx, media.Id, 0); err != nil {
		g.Log().Warningf(ctx, "直传媒体处理任务入队失败 media_id:%d err:%+v", media.Id, err)
	}
	return media, nil
}

func directUploadR2MD5(ctx context.Context, key string) (string, error) {
	obj, err := storager.R2GetObject(ctx, key)
	if err != nil {
		return "", gerror.Wrap(err, "读取R2直传文件失败")
	}
	defer obj.Close()
	hash := md5.New()
	if _, err = io.Copy(hash, obj); err != nil {
		return "", gerror.Wrap(err, "校验R2直传文件MD5失败")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func directUploadOssMD5(ctx context.Context, key string) (string, error) {
	obj, err := storager.OssGetObject(ctx, key)
	if err != nil {
		return "", gerror.Wrap(err, "读取OSS直传文件失败")
	}
	defer obj.Close()
	hash := md5.New()
	if _, err = io.Copy(hash, obj); err != nil {
		return "", gerror.Wrap(err, "校验OSS直传文件MD5失败")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func directUploadObjectMD5(ctx context.Context, client *cos.Client, key string) (string, error) {
	response, err := client.Object.Get(ctx, key, nil)
	if err != nil {
		return "", gerror.Wrap(err, "读取COS直传文件失败")
	}
	defer response.Body.Close()
	hash := md5.New()
	if _, err = io.Copy(hash, response.Body); err != nil {
		return "", gerror.Wrap(err, "校验COS直传文件MD5失败")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func promoteDirectUploadObject(ctx context.Context, client *cos.Client, cfg *model.UploadConfig, sourceKey, targetKey string, expectedSize int64) error {
	if existing, err := client.Object.Head(ctx, targetKey, nil); err == nil {
		if existing.Body != nil {
			_ = existing.Body.Close()
		}
		if err = validateDirectUploadObjectSize(existing.ContentLength, expectedSize); err != nil {
			return err
		}
		return nil
	} else if !cos.IsNotFoundError(err) {
		return gerror.Wrap(err, "检查COS内容对象失败")
	}
	bucketURL, err := url.Parse(strings.TrimSpace(cfg.CosBucketURL))
	if err != nil || bucketURL.Host == "" {
		return gerror.New("COS Bucket访问域名配置不正确")
	}
	contentType := mime.TypeByExtension("." + storager.Ext(targetKey))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, _, err = client.Object.Copy(ctx, targetKey, fmt.Sprintf("%s/%s", bucketURL.Host, strings.TrimLeft(sourceKey, "/")), &cos.ObjectCopyOptions{
		ObjectCopyHeaderOptions: &cos.ObjectCopyHeaderOptions{
			CacheControl:          "public, max-age=31536000, immutable",
			ContentType:           contentType,
			XCosMetadataDirective: "Replaced",
		},
	})
	if err != nil {
		return gerror.Wrap(err, "归档COS直传文件失败")
	}
	return nil
}

func validateDirectUploadObjectSize(actual, expected int64) error {
	if expected <= 0 || actual != expected {
		return gerror.Newf("COS内容对象大小不匹配，预期:%d 实际:%d", expected, actual)
	}
	return nil
}

func loadDirectUploadConfig(ctx context.Context) (*model.UploadConfig, error) {
	cfg, err := isc.SysConfig().GetUpload(ctx)
	if err != nil {
		return nil, gerror.Wrap(err, "读取媒体直传配置失败")
	}
	if cfg == nil {
		return nil, gerror.New("媒体直传配置不存在")
	}
	return cfg, nil
}

func validateDirectUploadConfig(ctx context.Context, session *mediaDirectUploadSession) (*model.UploadConfig, error) {
	cfg, err := loadDirectUploadConfig(ctx)
	if err != nil {
		return nil, err
	}
	var bucket, region string
	if cfg.Drive == consts.UploadDriveOss {
		bucket, region = cfg.OssBucket, "oss"
	} else if cfg.Drive == consts.UploadDriveR2 {
		bucket, region = cfg.R2Bucket, cfg.R2Region
		if region == "" {
			region = "auto"
		}
	} else {
		bucket, region, err = directUploadBucketRegionFromConfig(cfg.CosBucket, cfg.CosRegion, cfg.CosBucketURL, cfg.CosPublicURL)
		if err != nil {
			return nil, err
		}
	}
	if session.ConfigDigest == "" || session.ConfigDigest != directUploadConfigDigest(cfg) || session.Bucket != bucket || session.Region != region {
		return nil, gerror.New("对象存储配置已更新，请重新选择文件上传")
	}
	return cfg, nil
}

func directUploadConfigDigest(cfg *model.UploadConfig) string {
	if cfg == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(cfg.Drive), strings.TrimSpace(cfg.CosBucket), strings.TrimSpace(cfg.CosRegion),
		strings.TrimSpace(cfg.CosBucketURL), strings.TrimSpace(cfg.CosSecretId), strings.TrimSpace(cfg.CosSecretKey),
		strings.TrimSpace(cfg.OssEndpoint), strings.TrimSpace(cfg.OssBucketURL), strings.TrimSpace(cfg.OssBucket), strings.TrimSpace(cfg.OssSecretId), strings.TrimSpace(cfg.OssSecretKey), strings.TrimSpace(cfg.OssPath),
		strings.TrimSpace(cfg.R2Endpoint), strings.TrimSpace(cfg.R2Bucket), strings.TrimSpace(cfg.R2AccessKey), strings.TrimSpace(cfg.R2SecretKey),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
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

func (s *sSysPublish) mediaViewById(ctx context.Context, id int64) (*sysin.MediaModel, error) {
	res := new(sysin.MediaModel)
	if err := pdao.YoubanPublishMedia.Ctx(ctx).WherePri(id).Scan(res); err != nil {
		return nil, err
	}
	if res.Id <= 0 {
		return nil, gerror.New("直传媒体不存在")
	}
	normalizeMediaListFileURL([]*sysin.MediaModel{res})
	return res, nil
}
