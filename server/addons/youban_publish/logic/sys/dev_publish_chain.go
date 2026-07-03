package sys

import (
	"context"
	"crypto/md5"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/storager"
	"hotgo/internal/model/entity"
	basesysin "hotgo/internal/model/input/sysin"
)

var devDefaultTestFiles = []string{
	"/Users/ley/Downloads/Telegram Desktop/photo_2026-07-02_01-58-13.jpg",
	"/Users/ley/Downloads/Telegram Desktop/photo_2026-07-02_01-58-18.jpg",
	"/Users/ley/Downloads/Telegram Desktop/视频_20260612235233290_7.mp4",
}

func (s *sSysPublish) AdminDevPublishChainTest(ctx context.Context, in *sysin.DevPublishChainTestInp) (*sysin.DevPublishChainTestModel, error) {
	if !isDevelopMode(ctx) {
		return nil, gerror.New("开发测试接口仅允许在develop/testing环境使用")
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.devPublishChainTest(ctx, account, in)
}

func (s *sSysPublish) MyDevPublishChainTest(ctx context.Context, in *sysin.DevPublishChainTestInp) (*sysin.DevPublishChainTestModel, error) {
	if !isDevelopMode(ctx) {
		return nil, gerror.New("开发测试接口仅允许在develop/testing环境使用")
	}
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.devPublishChainTest(ctx, account, in)
}

func (s *sSysPublish) devPublishChainTest(ctx context.Context, account *sysin.AccountModel, in *sysin.DevPublishChainTestInp) (*sysin.DevPublishChainTestModel, error) {
	if in == nil {
		in = &sysin.DevPublishChainTestInp{}
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	files := in.FilePaths
	if len(files) == 0 {
		files = devDefaultTestFiles
	}
	if err := ensureLocalTestFiles(files); err != nil {
		return nil, err
	}
	channelIds, err := s.devTestChannelIds(ctx, account.TenantId, in.ChannelIds)
	if err != nil {
		return nil, err
	}
	res := &sysin.DevPublishChainTestModel{ChannelIds: channelIds, Items: []*sysin.DevPublishChainTestItem{}}
	for i := 0; i < in.Variants; i++ {
		item, itemErr := s.createDevPublishTestItem(ctx, account, in, files, channelIds, i)
		if itemErr != nil {
			return nil, itemErr
		}
		res.Items = append(res.Items, item)
	}
	return res, nil
}

func (s *sSysPublish) createDevPublishTestItem(ctx context.Context, account *sysin.AccountModel, in *sysin.DevPublishChainTestInp, files []string, channelIds []int64, index int) (*sysin.DevPublishChainTestItem, error) {
	publishAt := ""
	shouldSubmit := in.SubmitNow == 1
	if in.IncludeScheduled == 1 && index == in.Variants-1 {
		publishAt = gtime.Now().Add(time.Duration(in.ScheduledDelaySeconds) * time.Second).Format("Y-m-d H:i:s")
		shouldSubmit = false
	}
	title := devTestTitle(account.Nickname, in.TitleModes, index)
	saved, err := s.saveProfile(ctx, &sysin.ProfileSaveInp{
		ChannelIds:      channelIds,
		Title:           title,
		Province:        "420000",
		City:            "420100",
		PlainText:       devRandomText(index),
		Tag:             devTestTag(index),
		CustomerRemark:  "开发环境TG推送链路测试",
		AntiScanEnabled: index % 2,
		PublishAt:       publishAt,
		Status:          2,
	}, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	mediaIds, err := s.attachDevTestFiles(ctx, saved.TaskId, account.Id, files)
	if err != nil {
		return nil, err
	}
	if shouldSubmit {
		if err = s.submitTask(ctx, saved.TaskId, account.Id); err != nil {
			return nil, err
		}
	}
	return &sysin.DevPublishChainTestItem{
		MediaIds:  mediaIds,
		ProfileId: saved.Id,
		PublishAt: publishAt,
		Submitted: shouldSubmit,
		TaskId:    saved.TaskId,
		Title:     title,
	}, nil
}

func (s *sSysPublish) attachDevTestFiles(ctx context.Context, taskId int64, accountId int64, files []string) ([]int64, error) {
	task, err := s.getTask(ctx, taskId, accountId)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(files))
	for i, path := range files {
		mediaType := devMediaType(path)
		attachment, err := createDevLocalAttachment(ctx, path, mediaType)
		if err != nil {
			return nil, err
		}
		pHash := ""
		if mediaType == "image" {
			pHash, err = localImagePHash(path)
			if err != nil {
				return nil, err
			}
		}
		media, err := s.saveMediaAttachment(ctx, task, &sysin.MediaUploadInp{
			TaskId:    taskId,
			MediaType: mediaType,
			Purpose:   devMediaPurpose(mediaType),
			SortIndex: i + 1,
		}, attachment, nil, pHash)
		if err != nil {
			return nil, err
		}
		ids = append(ids, media.Id)
	}
	return ids, nil
}

func (s *sSysPublish) devTestChannelIds(ctx context.Context, tenantId int64, ids []int64) ([]int64, error) {
	if len(ids) > 0 {
		if err := s.ensureProfileChannels(ctx, ids, tenantId); err != nil {
			return nil, err
		}
		return ids, nil
	}
	var rows []gdb.Record
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		Where("status", 1).
		WhereNull("deleted_at").
		OrderDesc("is_default_selected").
		OrderAsc("id").
		Limit(3).
		Fields("id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取测试上架频道失败")
	}
	ids = make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["id"].Int64())
	}
	if len(ids) == 0 {
		return nil, gerror.New("请先配置可用的上架频道")
	}
	return ids, nil
}

func createDevLocalAttachment(ctx context.Context, path string, mediaType string) (*basesysin.AttachmentListModel, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, gerror.Wrap(err, "读取测试文件失败")
	}
	md5Value, err := fileMD5(path)
	if err != nil {
		return nil, err
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	mimeType := mime.TypeByExtension("." + ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	kind := storager.KindImg
	if mediaType == "video" {
		kind = storager.KindVideo
	}
	now := gtime.Now()
	columns := dao.SysAttachment.Columns()
	id, err := dao.SysAttachment.Ctx(ctx).Data(g.Map{
		columns.AppId:     consts.AppApi,
		columns.MemberId:  contexts.GetUserId(ctx),
		columns.Drive:     "local",
		columns.Name:      filepath.Base(path),
		columns.Kind:      kind,
		columns.MimeType:  mimeType,
		columns.NaiveType: mimeType,
		columns.Path:      path,
		columns.FileUrl:   "",
		columns.Size:      stat.Size(),
		columns.Ext:       ext,
		columns.Md5:       md5Value,
		columns.Status:    1,
		columns.CreatedAt: now,
		columns.UpdatedAt: now,
	}).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "创建测试附件失败")
	}
	return &basesysin.AttachmentListModel{
		SysAttachment: entity.SysAttachment{
			Id:        id,
			AppId:     consts.AppApi,
			MemberId:  contexts.GetUserId(ctx),
			Drive:     "local",
			Name:      filepath.Base(path),
			Kind:      kind,
			MimeType:  mimeType,
			NaiveType: mimeType,
			Path:      path,
			FileUrl:   "",
			Size:      stat.Size(),
			Ext:       ext,
			Md5:       md5Value,
			Status:    1,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}, nil
}

func ensureLocalTestFiles(files []string) error {
	for _, path := range files {
		if _, err := os.Stat(path); err != nil {
			return gerror.Wrapf(err, "测试文件不存在：%s", path)
		}
	}
	return nil
}

func fileMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", gerror.Wrap(err, "打开测试文件失败")
	}
	defer file.Close()
	hash := md5.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", gerror.Wrap(err, "计算测试文件MD5失败")
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func localImagePHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", gerror.Wrap(err, "打开测试图片失败")
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return "", gerror.New("测试图片格式不支持")
	}
	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return "", gerror.Wrap(err, "计算测试图片感知哈希失败")
	}
	return fmt.Sprintf("%016x", hash.GetHash()), nil
}

func devMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".mp4" || ext == ".mov" || ext == ".m4v" {
		return "video"
	}
	return "image"
}

func devMediaPurpose(mediaType string) string {
	if mediaType == "video" {
		return "verify"
	}
	return "display"
}

func devTestTitle(accountName string, modes []string, index int) string {
	if accountName == "" {
		accountName = "测试账号"
	}
	defaultModes := []string{"account_seq", "city_tag_seq", "tag_account_seq"}
	if len(modes) == 0 {
		modes = defaultModes
	}
	mode := modes[index%len(modes)]
	seq := fmt.Sprintf("%03d", index+1)
	switch mode {
	case "city_tag_seq":
		return "武汉相亲测试" + seq
	case "tag_account_seq":
		return "开发测试" + accountName + seq
	default:
		return accountName + seq
	}
}

func devRandomText(index int) string {
	samples := []string{
		"测试文案：认真生活，喜欢运动和旅行，希望遇到聊得来的人。",
		"测试文案：工作稳定，性格温和，周末喜欢看展和散步。",
		"测试文案：这是TG推送链路自动测试资料，用于验证图文、视频和发送记录。",
	}
	return samples[index%len(samples)]
}

func devTestTag(index int) string {
	tags := []string{"开发测试,温和", "武汉,运动", "视频验证,资料测试"}
	return tags[index%len(tags)]
}

func isDevelopMode(ctx context.Context) bool {
	mode := strings.TrimSpace(g.Cfg().MustGet(ctx, "system.mode", "develop").String())
	return mode == "" || mode == "not-set" || mode == "develop" || mode == "testing"
}
