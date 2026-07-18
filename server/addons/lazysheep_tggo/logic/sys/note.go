// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/hgrds/lock"
	"hotgo/internal/library/storager"
	isysin "hotgo/internal/model/input/sysin"
	isc "hotgo/internal/service"
	"hotgo/utility/file"
)

const (
	noteTypeTitle    = "NOTE_TYPE_TITLE"
	noteTypeText     = "NOTE_TYPE_TEXT"
	noteTypeImage    = "NOTE_TYPE_IMAGE"
	noteTypeVideo    = "NOTE_TYPE_VIDEO"
	noteTypeLocation = "NOTE_TYPE_LOCATION"
)

type sourceMessage struct {
	Content        string          `json:"content"`
	ContentId      string          `json:"contentId"`
	Id             string          `json:"id"`
	PairId         string          `json:"pairId"`
	ReceiverRoomId string          `json:"receiverRoomId"`
	RoomName       string          `json:"roomName"`
	Sender         string          `json:"sender"`
	SenderDno      string          `json:"senderDno"`
	SenderUser     json.RawMessage `json:"senderUser"`
	Type           string          `json:"type"`
	UpId           string          `json:"upId"`
}

type noteContent struct {
	UpId       int64      `json:"upId"`
	Items      []noteItem `json:"items"`
	CreateTime int64      `json:"createTime"`
	UpdateTime int64      `json:"updateTime"`
	TopTime    int64      `json:"topTime"`
}

type noteItem struct {
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	SubTitle    string  `json:"subTitle"`
	Content     string  `json:"content"`
	Duration    int     `json:"duration"`
	VerifyVideo bool    `json:"verifyVideo"`
	AspectRatio float64 `json:"aspectRatio"`
	TgFileID    string  `json:"tgFileId"`
}

type preparedNoteItem struct {
	Item       noteItem
	Attachment *isysin.AttachmentListModel
	MediaPHash string
}

func (s *sLazySheepTGGo) storeNote(ctx context.Context, in *sysin.NoteStoreInp) (res *sysin.NoteStoreModel, err error) {
	if in == nil || strings.TrimSpace(in.Payload) == "" {
		return nil, gerror.New("笔记原始内容不能为空")
	}
	var msg sourceMessage
	if err = json.Unmarshal([]byte(in.Payload), &msg); err != nil {
		return nil, gerror.Wrap(err, "解析消息失败")
	}
	if msg.Type != "MESSAGE_TYPE_NOTES" {
		return nil, gerror.New("仅支持 MESSAGE_TYPE_NOTES 类型")
	}
	var note noteContent
	if err = json.Unmarshal([]byte(msg.Content), &note); err != nil {
		return nil, gerror.Wrap(err, "解析笔记内容失败")
	}

	title, text := noteText(note.Items)
	botID, err := s.resolveBotID(ctx, in.BotKey)
	if err != nil {
		return nil, err
	}
	bindingID, err := s.resolveBindingID(ctx, in.BindingKey)
	if err != nil {
		return nil, err
	}
	contentID := parseInt(msg.ContentId)
	preparedItems, err := s.prepareNoteItems(ctx, note.Items)
	if err != nil {
		return nil, err
	}
	code, err := s.genBindingNoteCode(ctx, botID, bindingID, contentID, msg.Id)
	if err != nil {
		return nil, err
	}
	noteLock := lock.NewConfig(2*time.Minute, 200*time.Millisecond).Mutex(fmt.Sprintf("lazysheep_tggo:note:store:%s:%d", in.BindingKey, contentID))
	if err = noteLock.Lock(ctx); err != nil {
		return nil, gerror.Wrap(err, "等待笔记入库锁失败")
	}
	defer func() {
		if unlockErr := noteLock.Unlock(context.Background()); unlockErr != nil {
			g.Log().Warningf(ctx, "释放笔记入库锁失败 binding:%s contentID:%d err:%+v", in.BindingKey, contentID, unlockErr)
		}
	}()

	err = dao.AddonLazysheepTggoNote.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		noteID, err := s.upsertNoteRow(ctx, &noteStoreRow{
			BotID:          botID,
			BindingID:      bindingID,
			ContentID:      contentID,
			UpID:           note.UpId,
			PairID:         msg.PairId,
			ReceiverRoomID: parseInt(msg.ReceiverRoomId),
			RoomName:       msg.RoomName,
			Sender:         msg.Sender,
			SenderDno:      msg.SenderDno,
			SenderUser:     msg.SenderUser,
			RawPayload:     in.Payload,
			NotePayload:    msg.Content,
			Code:           code,
			Title:          title,
			TextContent:    text,
		})
		if err != nil {
			return err
		}
		if err = s.replaceNoteItems(ctx, noteID, botID, preparedItems); err != nil {
			return err
		}
		res = &sysin.NoteStoreModel{NoteId: noteID, Code: code}
		return nil
	})
	return
}

func (s *sLazySheepTGGo) prepareNoteItems(ctx context.Context, items []noteItem) ([]preparedNoteItem, error) {
	hasImage := false
	for _, item := range items {
		if item.Type == noteTypeImage {
			hasImage = true
			break
		}
	}
	if hasImage {
		if err := s.ensureNoteAssetPHashField(ctx); err != nil {
			g.Log().Warningf(ctx, "确保笔记资源感知哈希字段失败 err:%+v", err)
		}
	}
	prepared := make([]preparedNoteItem, 0, len(items))
	for _, item := range items {
		row := preparedNoteItem{
			Item: item,
		}
		if item.Type == noteTypeImage {
			row.MediaPHash = mediaPHashFromAttachmentOrSource(ctx, nil, item.Content, item.Type)
		}
		prepared = append(prepared, row)
	}
	return prepared, nil
}

type noteStoreRow struct {
	BotID          int
	BindingID      int
	ContentID      int64
	UpID           int64
	PairID         string
	ReceiverRoomID int64
	RoomName       string
	Sender         string
	SenderDno      string
	SenderUser     json.RawMessage
	RawPayload     string
	NotePayload    string
	Code           string
	Title          string
	TextContent    string
}

func (s *sLazySheepTGGo) upsertNoteRow(ctx context.Context, row *noteStoreRow) (int64, error) {
	cols := dao.AddonLazysheepTggoNote.Columns()
	data := g.Map{
		cols.BotId:          row.BotID,
		cols.BindingId:      row.BindingID,
		cols.ContentId:      row.ContentID,
		cols.UpId:           row.UpID,
		cols.PairId:         row.PairID,
		cols.ReceiverRoomId: row.ReceiverRoomID,
		cols.RoomName:       row.RoomName,
		cols.Sender:         row.Sender,
		cols.SenderDno:      row.SenderDno,
		cols.SenderUser:     gjson.New(row.SenderUser),
		cols.RawPayload:     gjson.New(row.RawPayload),
		cols.NotePayload:    gjson.New(row.NotePayload),
		cols.MessageType:    "MESSAGE_TYPE_NOTES",
		cols.Code:           row.Code,
		cols.Title:          row.Title,
		cols.TextContent:    row.TextContent,
		cols.WorkflowStatus: 1,
		cols.Status:         1,
	}
	return upsertNoteByBindingContent(ctx, row.BindingID, row.ContentID, data)
}

func upsertNoteByBindingContent(ctx context.Context, bindingID int, contentID int64, row g.Map) (int64, error) {
	cols := dao.AddonLazysheepTggoNote.Columns()
	mod := dao.AddonLazysheepTggoNote.Ctx(ctx)
	existing, err := mod.Clone().
		Unscoped().
		Fields(cols.Id).
		Where(cols.BindingId, bindingID).
		Where(cols.ContentId, contentID).
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "查询记录失败")
	}
	row[cols.DeletedAt] = nil
	if !existing.IsNil() {
		if _, err = mod.Clone().Unscoped().Where(cols.Id, existing.Int64()).Data(row).Update(); err != nil {
			return 0, gerror.Wrap(err, "更新记录失败")
		}
		return existing.Int64(), nil
	}
	id, err := mod.Clone().Data(row).InsertAndGetId()
	if err == nil {
		return id, nil
	}
	existing, lookupErr := mod.Clone().
		Unscoped().
		Fields(cols.Id).
		Where(cols.BindingId, bindingID).
		Where(cols.ContentId, contentID).
		Value()
	if lookupErr == nil && !existing.IsNil() {
		if _, updateErr := mod.Clone().Unscoped().Where(cols.Id, existing.Int64()).Data(row).Update(); updateErr != nil {
			return 0, gerror.Wrap(updateErr, "更新记录失败")
		}
		return existing.Int64(), nil
	}
	return 0, gerror.Wrap(err, "新增记录失败")
}

func (s *sLazySheepTGGo) replaceNoteItems(ctx context.Context, noteID int64, botID int, items []preparedNoteItem) error {
	cols := dao.AddonLazysheepTggoNoteItem.Columns()
	if _, err := dao.AddonLazysheepTggoNoteItem.Ctx(ctx).Where(cols.NoteId, noteID).Delete(); err != nil {
		return gerror.Wrap(err, "清理旧笔记项失败")
	}
	assetCols := dao.AddonLazysheepTggoNoteAsset.Columns()
	if _, err := dao.AddonLazysheepTggoNoteAsset.Ctx(ctx).Where(assetCols.NoteId, noteID).Delete(); err != nil {
		return gerror.Wrap(err, "清理旧笔记资源失败")
	}
	for index, prepared := range items {
		item := prepared.Item
		row := g.Map{
			cols.NoteId:      noteID,
			cols.ItemIndex:   index,
			cols.ItemType:    item.Type,
			cols.Title:       item.Title,
			cols.SubTitle:    item.SubTitle,
			cols.Content:     item.Content,
			cols.Duration:    item.Duration,
			cols.AspectRatio: item.AspectRatio,
			cols.VerifyVideo: boolToInt(item.VerifyVideo),
			cols.Status:      1,
		}
		if prepared.Attachment != nil {
			row[cols.AttachmentId] = prepared.Attachment.Id
			row[cols.PreviewUrl] = prepared.Attachment.FileUrl
			row[cols.LocalPath] = prepared.Attachment.Path
		}
		itemID, err := dao.AddonLazysheepTggoNoteItem.Ctx(ctx).Data(row).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "保存笔记项失败")
		}
		if isRemoteMedia(item.Type) {
			if err = s.insertNoteAsset(ctx, noteID, int64(botID), itemID, index, prepared); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *sLazySheepTGGo) insertNoteAsset(ctx context.Context, noteID int64, botID int64, itemID int64, index int, prepared preparedNoteItem) error {
	item := prepared.Item
	cols := dao.AddonLazysheepTggoNoteAsset.Columns()
	assetType := "image"
	if item.Type == noteTypeVideo {
		assetType = "video"
		if item.VerifyVideo {
			assetType = "verify_video"
		}
	}
	row := g.Map{
		cols.NoteId:        noteID,
		cols.BotId:         botID,
		cols.ItemId:        itemID,
		cols.AssetType:     assetType,
		cols.SourceUrl:     item.Content,
		cols.Duration:      item.Duration,
		cols.AspectRatio:   item.AspectRatio,
		cols.ConvertStatus: 1,
		cols.Sort:          index,
		cols.Status:        1,
	}
	if prepared.Attachment != nil {
		row[cols.AttachmentId] = prepared.Attachment.Id
		row[cols.PreviewUrl] = prepared.Attachment.FileUrl
		row[cols.LocalPath] = prepared.Attachment.Path
	}
	if assetType == "image" && prepared.MediaPHash != "" {
		row["media_phash"] = prepared.MediaPHash
	}
	if _, err := dao.AddonLazysheepTggoNoteAsset.Ctx(ctx).Data(row).Insert(); err != nil {
		return gerror.Wrap(err, "保存笔记资源失败")
	}
	return nil
}

func mediaPHashFromAttachmentOrSource(ctx context.Context, attachment *isysin.AttachmentListModel, sourceURL string, itemType string) string {
	if attachment != nil {
		if phash := mediaPHashFromLocalPath(attachment.Path); phash != "" {
			return phash
		}
	}
	_, data, _, err := downloadCachedMedia(ctx, sourceURL, itemType, 0)
	if err != nil {
		g.Log().Warningf(ctx, "计算图片感知哈希下载失败 url:%s err:%+v", sourceURL, err)
		return ""
	}
	return mediaPHashFromBytes(data)
}

func (s *sLazySheepTGGo) StoreNote(ctx context.Context, in *sysin.NoteStoreInp) (res *sysin.NoteStoreModel, err error) {
	return s.storeNote(ctx, in)
}

func (s *sLazySheepTGGo) resolveBindingID(ctx context.Context, bindingKey string) (int, error) {
	if strings.TrimSpace(bindingKey) == "" {
		return 0, nil
	}
	cols := dao.AddonLazysheepTggoBinding.Columns()
	val, err := dao.AddonLazysheepTggoBinding.Ctx(ctx).Fields(cols.Id).Where(cols.BindingKey, bindingKey).Value()
	if err != nil {
		return 0, gerror.Wrap(err, "查询绑定失败")
	}
	if val.IsNil() {
		return 0, gerror.Newf("绑定不存在：%s", bindingKey)
	}
	return val.Int(), nil
}

func transferRemoteMedia(ctx context.Context, itemType, rawURL string) (*isysin.AttachmentListModel, error) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return nil, gerror.New("仅支持 HTTP/HTTPS 媒体链接")
	}
	if attachment, err := cachedAttachmentBySourceURL(ctx, rawURL); err != nil {
		return nil, err
	} else if attachment != nil {
		g.Log().Debugf(ctx, "命中 BangChat 媒体附件缓存 url:%s attachmentId:%d", rawURL, attachment.Id)
		return attachment, nil
	}
	filename, content, contentType, err := downloadCachedMedia(ctx, rawURL, itemType, 0)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, gerror.New("媒体内容为空")
	}
	kind, ext := mediaKindAndExt(itemType, rawURL, contentType)
	name := strings.TrimSpace(filename)
	if name == "" || path.Ext(name) == "" {
		name = "lazy-" + shortHash(rawURL) + ext
	}
	fileHeader, err := file.NewMultipartFileHeader(name, content)
	if err != nil {
		return nil, gerror.Newf("创建文件头失败：%v", err)
	}
	return isc.CommonUpload().UploadFile(ctx, kind, &ghttp.UploadFile{FileHeader: fileHeader})
}

func mediaKindAndExt(itemType, rawURL, contentType string) (string, string) {
	urlExt := strings.ToLower(path.Ext(strings.Split(rawURL, "?")[0]))
	if urlExt == "" {
		if strings.Contains(contentType, "video/") {
			urlExt = ".mp4"
		} else {
			urlExt = ".jpg"
		}
	}
	if itemType == noteTypeVideo {
		return storager.KindVideo, urlExt
	}
	return storager.KindImg, urlExt
}

func noteText(items []noteItem) (title string, text string) {
	var parts []string
	for _, item := range items {
		switch item.Type {
		case noteTypeTitle:
			if title == "" {
				title = item.Content
			}
		case noteTypeText:
			if strings.TrimSpace(item.Content) != "" {
				parts = append(parts, item.Content)
			}
		}
	}
	return title, strings.Join(parts, "\n")
}

func parseInt(v string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}

func genNoteCode(contentID int64, fallback string) string {
	raw := contentID
	if raw <= 0 {
		raw = parseInt(fallback)
	}
	if raw < 0 {
		raw = -raw
	}
	seed := crc32.ChecksumIEEE([]byte(fmt.Sprintf("%d:%s", raw, fallback)))
	letters := []byte{'A' + byte(seed%26), 'A' + byte((seed/26)%26)}
	return fmt.Sprintf("%s%07d", string(letters), raw%10000000)
}

func (s *sLazySheepTGGo) genBindingNoteCode(ctx context.Context, botID int, bindingID int, contentID int64, fallback string) (string, error) {
	cols := dao.AddonLazysheepTggoNote.Columns()
	existing, err := dao.AddonLazysheepTggoNote.Ctx(ctx).
		Unscoped().
		Fields(cols.Code).
		Where(cols.BindingId, bindingID).
		Where(cols.ContentId, contentID).
		Value()
	if err != nil {
		return "", gerror.Wrap(err, "查询已有笔记编号失败")
	}
	if !existing.IsNil() && strings.TrimSpace(existing.String()) != "" {
		return existing.String(), nil
	}
	base := genNoteCode(contentID, fallback)
	code := base
	for i := 0; i < 20; i++ {
		val, err := dao.AddonLazysheepTggoNote.Ctx(ctx).
			Unscoped().
			Fields(cols.Id).
			Where(cols.BotId, botID).
			Where(cols.BindingId, bindingID).
			Where(cols.Code, code).
			Value()
		if err != nil {
			return "", gerror.Wrap(err, "检查笔记编号失败")
		}
		if val.IsNil() {
			return code, nil
		}
		code = fmt.Sprintf("%s%07d", base[:2], (parseInt(base[2:])+int64(i)+1)%10000000)
	}
	return "", gerror.New("生成笔记编号失败")
}

func isRemoteMedia(itemType string) bool {
	return itemType == noteTypeImage || itemType == noteTypeVideo
}
