package sys

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
)

const sysAttachmentTable = "hg_sys_attachment"

var (
	attachmentVideoMetaSchemaMu    sync.Mutex
	attachmentVideoMetaSchemaReady bool
)

type telegramVideoMeta struct {
	Width    int
	Height   int
	Duration int
}

type ffprobeVideoMetaResult struct {
	Streams []struct {
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Duration string `json:"duration"`
		Tags     struct {
			Rotate string `json:"rotate"`
		} `json:"tags"`
		SideDataList []struct {
			Rotation int `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func (s *sSysPublish) telegramVideoMeta(ctx context.Context, media *telegramMediaItem) telegramVideoMeta {
	if media == nil || strings.TrimSpace(media.MediaType) != "video" {
		return telegramVideoMeta{}
	}
	if media.VideoWidth > 0 && media.VideoHeight > 0 {
		return telegramVideoMeta{Width: media.VideoWidth, Height: media.VideoHeight, Duration: media.VideoDuration}
	}
	if err := ensureAttachmentVideoMetaSchema(ctx); err != nil {
		g.Log().Warningf(ctx, "检查资源视频元数据字段失败: %+v", err)
		return telegramVideoMeta{}
	}
	if meta, ok := attachmentVideoMetaFromCache(ctx, media); ok {
		media.VideoWidth = meta.Width
		media.VideoHeight = meta.Height
		media.VideoDuration = meta.Duration
		return meta
	}
	localPath, cleanup, err := telegramVideoMetaProbePath(ctx, media)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		g.Log().Warningf(ctx, "准备读取视频元数据失败 mediaId:%d attachmentId:%d path:%s err:%+v", media.Id, media.AttachmentId, media.StoragePath, err)
		return telegramVideoMeta{}
	}
	if strings.TrimSpace(localPath) == "" {
		return telegramVideoMeta{}
	}
	meta, err := probeLocalVideoMeta(ctx, localPath)
	if err != nil {
		g.Log().Warningf(ctx, "读取视频元数据失败 mediaId:%d attachmentId:%d path:%s err:%+v", media.Id, media.AttachmentId, localPath, err)
		return telegramVideoMeta{}
	}
	if meta.Width <= 0 || meta.Height <= 0 {
		return telegramVideoMeta{}
	}
	media.VideoWidth = meta.Width
	media.VideoHeight = meta.Height
	media.VideoDuration = meta.Duration
	if err = saveAttachmentVideoMeta(ctx, media, meta); err != nil {
		g.Log().Warningf(ctx, "缓存视频元数据失败 mediaId:%d attachmentId:%d path:%s err:%+v", media.Id, media.AttachmentId, media.StoragePath, err)
	}
	return meta
}

func (s *sSysPublish) prepareTelegramMediaItemsForSend(ctx context.Context, media []*telegramMediaItem) {
	for _, item := range media {
		s.prepareTelegramMediaItemForSend(ctx, item)
	}
}

func (s *sSysPublish) prepareTelegramMediaItemForSend(ctx context.Context, media *telegramMediaItem) {
	if media == nil {
		return
	}
	mediaType := strings.ToLower(strings.TrimSpace(media.MediaType))
	if mediaType == "video" {
		_ = s.telegramVideoMeta(ctx, media)
	}
	if telegramMediaRequiresSanitizedUpload(media) {
		media.TgFileId = ""
		media.TgThumbFileId = ""
	}
}

func telegramVideoMetaProbePath(ctx context.Context, media *telegramMediaItem) (string, func(), error) {
	if media == nil {
		return "", nil, nil
	}
	return cachedTelegramMediaFile(ctx, media)
}

func applyTelegramSendVideoMeta(params *tgbot.SendVideoParams, meta telegramVideoMeta) {
	if params == nil {
		return
	}
	if meta.Width > 0 {
		params.Width = meta.Width
	}
	if meta.Height > 0 {
		params.Height = meta.Height
	}
	if meta.Duration > 0 {
		params.Duration = meta.Duration
	}
}

func applyTelegramInputMediaVideoMeta(video *models.InputMediaVideo, meta telegramVideoMeta) {
	if video == nil {
		return
	}
	if meta.Width > 0 {
		video.Width = meta.Width
	}
	if meta.Height > 0 {
		video.Height = meta.Height
	}
	if meta.Duration > 0 {
		video.Duration = meta.Duration
	}
}

func ensureAttachmentVideoMetaSchema(ctx context.Context) error {
	attachmentVideoMetaSchemaMu.Lock()
	defer attachmentVideoMetaSchemaMu.Unlock()
	if attachmentVideoMetaSchemaReady {
		return nil
	}
	var err error
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		err = ensureAttachmentVideoMetaPgsqlSchema(ctx)
	} else {
		err = ensureAttachmentVideoMetaMysqlSchema(ctx)
	}
	if err != nil {
		return err
	}
	attachmentVideoMetaSchemaReady = true
	return nil
}

func ensureAttachmentVideoMetaPgsqlSchema(ctx context.Context) error {
	statements := []string{
		`ALTER TABLE "hg_sys_attachment" ADD COLUMN IF NOT EXISTS "width" integer NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_sys_attachment" ADD COLUMN IF NOT EXISTS "height" integer NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_sys_attachment" ADD COLUMN IF NOT EXISTS "duration" integer NOT NULL DEFAULT 0`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "检查资源视频元数据字段失败")
		}
	}
	return nil
}

func ensureAttachmentVideoMetaMysqlSchema(ctx context.Context) error {
	statements := []string{
		"ALTER TABLE `hg_sys_attachment` ADD COLUMN `width` int NOT NULL DEFAULT 0 COMMENT '宽度'",
		"ALTER TABLE `hg_sys_attachment` ADD COLUMN `height` int NOT NULL DEFAULT 0 COMMENT '高度'",
		"ALTER TABLE `hg_sys_attachment` ADD COLUMN `duration` int NOT NULL DEFAULT 0 COMMENT '时长'",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil && !isIgnorableAttachmentVideoMetaSchemaError(err) {
			return gerror.Wrap(err, "检查资源视频元数据字段失败")
		}
	}
	return nil
}

func isIgnorableAttachmentVideoMetaSchemaError(err error) bool {
	if err == nil {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already exists") ||
		strings.Contains(message, "duplicate column")
}

func attachmentVideoMetaFromCache(ctx context.Context, media *telegramMediaItem) (telegramVideoMeta, bool) {
	if media == nil {
		return telegramVideoMeta{}, false
	}
	mod := g.DB().Model(sysAttachmentTable).Safe().Ctx(ctx).Fields("id,width,height,duration")
	if media.AttachmentId > 0 {
		mod = mod.Where("id", media.AttachmentId)
	} else if path := strings.TrimSpace(media.StoragePath); path != "" {
		mod = mod.Where("path", path)
	} else {
		return telegramVideoMeta{}, false
	}
	row, err := mod.One()
	if err != nil || row == nil || row.IsEmpty() {
		if err != nil {
			g.Log().Warningf(ctx, "读取资源视频元数据缓存失败 attachmentId:%d path:%s err:%+v", media.AttachmentId, media.StoragePath, err)
		}
		return telegramVideoMeta{}, false
	}
	meta := telegramVideoMeta{
		Width:    row["width"].Int(),
		Height:   row["height"].Int(),
		Duration: row["duration"].Int(),
	}
	return meta, meta.Width > 0 && meta.Height > 0
}

func saveAttachmentVideoMeta(ctx context.Context, media *telegramMediaItem, meta telegramVideoMeta) error {
	if media == nil || meta.Width <= 0 || meta.Height <= 0 {
		return nil
	}
	data := g.Map{
		"width":      meta.Width,
		"height":     meta.Height,
		"duration":   meta.Duration,
		"updated_at": gtime.Now(),
	}
	mod := g.DB().Model(sysAttachmentTable).Safe().Ctx(ctx).Data(data)
	if media.AttachmentId > 0 {
		mod = mod.Where("id", media.AttachmentId)
	} else if path := strings.TrimSpace(media.StoragePath); path != "" {
		mod = mod.Where("path", path)
	} else {
		return nil
	}
	_, err := mod.Update()
	return err
}

func probeLocalVideoMeta(ctx context.Context, localPath string) (telegramVideoMeta, error) {
	localPath = strings.TrimSpace(localPath)
	if localPath == "" {
		return telegramVideoMeta{}, nil
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return telegramVideoMeta{}, gerror.New("ffprobe 未安装，无法读取视频元数据")
	}
	cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(
		cmdCtx,
		ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration,tags:stream_side_data=rotation:format=duration",
		"-of", "json",
		localPath,
	).CombinedOutput()
	if err != nil {
		return telegramVideoMeta{}, gerror.Wrapf(err, "ffprobe 读取失败：%s", ellipsisString(strings.TrimSpace(string(output)), 500))
	}
	var result ffprobeVideoMetaResult
	if err = json.Unmarshal(output, &result); err != nil {
		return telegramVideoMeta{}, gerror.Wrap(err, "解析 ffprobe 输出失败")
	}
	if len(result.Streams) == 0 {
		return telegramVideoMeta{}, nil
	}
	stream := result.Streams[0]
	width := stream.Width
	height := stream.Height
	if shouldSwapVideoSize(stream.Tags.Rotate, stream.SideDataList) {
		width, height = height, width
	}
	duration := parseProbeDurationSeconds(stream.Duration)
	if duration <= 0 {
		duration = parseProbeDurationSeconds(result.Format.Duration)
	}
	return telegramVideoMeta{Width: width, Height: height, Duration: duration}, nil
}

func shouldSwapVideoSize(tagRotate string, sideData []struct {
	Rotation int `json:"rotation"`
}) bool {
	rotation := 0
	if strings.TrimSpace(tagRotate) != "" {
		rotation, _ = strconv.Atoi(strings.TrimSpace(tagRotate))
	}
	for _, item := range sideData {
		if item.Rotation != 0 {
			rotation = item.Rotation
			break
		}
	}
	rotation = ((rotation % 360) + 360) % 360
	return rotation == 90 || rotation == 270
}

func parseProbeDurationSeconds(value string) int {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "N/A") {
		return 0
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return int(math.Round(seconds))
}
