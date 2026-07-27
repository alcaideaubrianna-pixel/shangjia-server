package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

type importRunMatchRunRow struct {
	sysin.ImportRunMatchRunModel
}

type importMatchTaskText struct {
	TaskId    int64
	ProfileId int64
	Title     string
	ProfileNo string
	SourceKey string
	PlainText string
}

type importMatchCandidateGroup struct {
	GroupKey       string
	MediaGroupId   string
	FirstMessageId int64
	LastMessageId  int64
	MessageDate    *gtime.Time
	CaptionText    string
	MediaCount     int
	MediaTypes     string
}

func (s *sSysPublish) ServerImportRunMatchConfig(ctx context.Context, in *sysin.ImportRunMatchConfigInp) (*sysin.ImportRunMatchConfigModel, error) {
	if in == nil || in.ImportRunId <= 0 {
		return nil, gerror.New("导入记录ID不能为空")
	}
	run, err := s.importRunMatchImportRun(ctx, in.ImportRunId)
	if err != nil {
		return nil, err
	}
	channels, err := s.importRunMatchChannels(ctx, run["tenant_id"].Int64(), nil)
	if err != nil {
		return nil, err
	}
	latest, err := s.latestImportRunMatch(ctx, in.ImportRunId)
	if err != nil {
		return nil, err
	}
	return &sysin.ImportRunMatchConfigModel{
		ImportRunId: in.ImportRunId,
		LatestRun:   latest,
		Channels:    channels,
	}, nil
}

func (s *sSysPublish) ServerImportRunMatchStart(ctx context.Context, in *sysin.ImportRunMatchStartInp) (*sysin.ImportRunMatchRunModel, error) {
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	run, err := s.importRunMatchImportRun(ctx, in.ImportRunId)
	if err != nil {
		return nil, err
	}
	channels, err := s.importRunMatchChannels(ctx, run["tenant_id"].Int64(), in.ChannelIds)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, gerror.New("未找到可匹配的频道")
	}
	matchRunId, err := s.createImportRunMatchRun(ctx, run, channels, 0, in.Threshold, "match_created")
	if err != nil {
		return nil, err
	}
	if err = s.enqueueImportMatchRun(ctx, matchRunId, 0); err != nil {
		return nil, gerror.Wrap(err, "导入TG匹配加入队列失败")
	}
	return s.ServerImportRunMatchView(ctx, &sysin.ImportRunMatchViewInp{Id: matchRunId})
}

func (s *sSysPublish) ServerImportRunTgSyncStart(ctx context.Context, in *sysin.ImportRunTgSyncStartInp) (*sysin.ImportRunMatchRunModel, error) {
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	run, err := s.importRunMatchImportRun(ctx, in.ImportRunId)
	if err != nil {
		return nil, err
	}
	channels, err := s.importRunMatchChannels(ctx, run["tenant_id"].Int64(), in.ChannelIds)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, gerror.New("未找到可同步的频道")
	}
	matchRunId, err := s.createImportRunMatchRun(ctx, run, channels, in.ScanDays, 0, "sync_created")
	if err != nil {
		return nil, err
	}
	if err = s.enqueueImportTgSyncRun(ctx, matchRunId, 0); err != nil {
		return nil, gerror.Wrap(err, "导入TG消息同步加入队列失败")
	}
	return s.ServerImportRunMatchView(ctx, &sysin.ImportRunMatchViewInp{Id: matchRunId})
}

func (s *sSysPublish) createImportRunMatchRun(ctx context.Context, run gdb.Record, channels []*sysin.ImportRunMatchChannelModel, scanDays int, threshold int, stage string) (int64, error) {
	channelIds := make([]int64, 0, len(channels))
	for _, channel := range channels {
		channelIds = append(channelIds, channel.Id)
	}
	channelJSON, _ := json.Marshal(channelIds)
	now := gtime.Now()
	matchRunId, err := g.DB().Model(publishImportMatchRunTable).Safe().Ctx(ctx).Data(g.Map{
		"import_run_id":   run["id"].Int64(),
		"tenant_id":       run["tenant_id"].Int64(),
		"account_id":      run["account_id"].Int64(),
		"status":          sysin.ImportMatchRunStatusPending,
		"stage":           stage,
		"channel_id_json": string(channelJSON),
		"scan_days":       scanDays,
		"threshold":       threshold,
		"created_at":      now,
		"updated_at":      now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建导入TG匹配运行失败")
	}
	return matchRunId, nil
}

func (s *sSysPublish) ServerImportRunMatchView(ctx context.Context, in *sysin.ImportRunMatchViewInp) (*sysin.ImportRunMatchRunModel, error) {
	if in == nil || (in.Id <= 0 && in.ImportRunId <= 0) {
		return nil, gerror.New("匹配运行ID不能为空")
	}
	mod := g.DB().Model(publishImportMatchRunTable).Safe().Ctx(ctx).WhereNull("deleted_at")
	if in.Id > 0 {
		mod = mod.Where("id", in.Id)
	} else {
		mod = mod.Where("import_run_id", in.ImportRunId).OrderDesc("id").Limit(1)
	}
	var item sysin.ImportRunMatchRunModel
	if err := mod.Scan(&item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gerror.New("导入TG匹配运行不存在")
		}
		return nil, gerror.Wrap(err, "读取导入TG匹配运行失败")
	}
	if item.Id <= 0 {
		return nil, gerror.New("导入TG匹配运行不存在")
	}
	item.ChannelIds = decodeInt64JSON(item.ChannelIdJson)
	return &item, nil
}

func (s *sSysPublish) ServerImportRunMatchItemList(ctx context.Context, in *sysin.ImportRunMatchItemListInp) ([]*sysin.ImportRunMatchItemModel, int, error) {
	if in == nil || in.MatchRunId <= 0 {
		return nil, 0, gerror.New("匹配运行ID不能为空")
	}
	mod := g.DB().Model(publishImportMatchItemTable+" i").Safe().Ctx(ctx).
		LeftJoin(publishChannelTable+" c", "c.id=i.channel_id").
		LeftJoin(dao.ContentProfile.Table()+" p", "p.id=i.profile_id AND p.deleted_at IS NULL").
		Where("i.match_run_id", in.MatchRunId).
		WhereNull("i.deleted_at")
	if strings.TrimSpace(in.Status) != "" {
		mod = mod.Where("i.match_status", strings.TrimSpace(in.Status))
	}
	if in.ChannelId > 0 {
		mod = mod.Where("i.channel_id", in.ChannelId)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(p.title LIKE ? OR p.profile_no LIKE ? OR p.source_key LIKE ? OR p.plain_text LIKE ?)", like, like, like, like)
	}
	total, err := mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计导入TG匹配资料失败")
	}
	if total == 0 {
		return []*sysin.ImportRunMatchItemModel{}, 0, nil
	}
	var list []*sysin.ImportRunMatchItemModel
	err = mod.Fields("i.*,c.channel_title AS channel_name,p.title,p.plain_text,p.profile_no,p.source_key").
		Page(in.Page, in.PerPage).
		OrderDesc("i.total_score").
		OrderAsc("i.id").
		Scan(&list)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "读取导入TG匹配资料失败")
	}
	return list, total, nil
}

func (s *sSysPublish) ServerImportRunMatchCandidateList(ctx context.Context, in *sysin.ImportRunMatchCandidateListInp) ([]*sysin.ImportRunMatchCandidateModel, error) {
	if in == nil || in.ItemId <= 0 {
		return nil, gerror.New("匹配项ID不能为空")
	}
	item, err := s.importRunMatchItem(ctx, in.ItemId)
	if err != nil {
		return nil, err
	}
	taskText, err := s.importMatchTaskText(ctx, item.ProfileId)
	if err != nil {
		return nil, err
	}
	var list []*sysin.ImportRunMatchCandidateModel
	err = g.DB().Model(publishImportMatchCandidateTable).Safe().Ctx(ctx).
		Where("match_run_id", item.MatchRunId).
		Where("channel_id", item.ChannelId).
		WhereNull("deleted_at").
		OrderDesc("message_date").
		Limit(500).
		Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "读取导入TG匹配候选失败")
	}
	for _, candidate := range list {
		candidate.Score = importMatchSimilarityScore(taskText, candidate.CaptionText)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Score == list[j].Score {
			if list[i].MessageDate == nil || list[j].MessageDate == nil {
				return list[i].Id < list[j].Id
			}
			return list[i].MessageDate.Time.After(list[j].MessageDate.Time)
		}
		return list[i].Score > list[j].Score
	})
	return list, nil
}

func (s *sSysPublish) ServerImportRunMatchCandidateSearch(ctx context.Context, in *sysin.ImportRunMatchCandidateSearchInp) ([]*sysin.ImportRunMatchCandidateModel, int, error) {
	if in == nil || in.ItemId <= 0 {
		return nil, 0, gerror.New("匹配项ID不能为空")
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
	item, err := s.importRunMatchItem(ctx, in.ItemId)
	if err != nil {
		return nil, 0, err
	}
	taskText, err := s.importMatchTaskText(ctx, item.ProfileId)
	if err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(publishImportMatchCandidateTable).Safe().Ctx(ctx).
		Where("match_run_id", item.MatchRunId).
		Where("channel_id", item.ChannelId).
		WhereNull("deleted_at")
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(caption_text LIKE ? OR group_key LIKE ? OR media_group_id LIKE ?)", like, like, like)
	}
	total, err := mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计TG候选消息失败")
	}
	if total == 0 {
		return []*sysin.ImportRunMatchCandidateModel{}, 0, nil
	}
	var list []*sysin.ImportRunMatchCandidateModel
	err = mod.Page(in.Page, in.PerPage).
		OrderDesc("message_date").
		OrderDesc("first_message_id").
		Scan(&list)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "搜索TG候选消息失败")
	}
	for _, candidate := range list {
		candidate.Score = importMatchSimilarityScore(taskText, candidate.CaptionText)
	}
	return list, total, nil
}

func (s *sSysPublish) ServerImportRunMatchSaveDraft(ctx context.Context, in *sysin.ImportRunMatchSaveDraftInp) error {
	if in == nil || in.ItemId <= 0 {
		return gerror.New("匹配项ID不能为空")
	}
	item, err := s.importRunMatchItem(ctx, in.ItemId)
	if err != nil {
		return err
	}
	if err = s.validateImportMatchCandidateKeys(ctx, item, in.DisplayGroupKey, in.VerifyGroupKey); err != nil {
		return err
	}
	status := sysin.ImportMatchItemStatusManualPending
	if strings.TrimSpace(in.DisplayGroupKey) != "" && strings.TrimSpace(in.VerifyGroupKey) != "" {
		status = sysin.ImportMatchItemStatusAutoSelected
	}
	_, err = g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
		Where("id", in.ItemId).
		Data(g.Map{
			"display_group_key": strings.TrimSpace(in.DisplayGroupKey),
			"verify_group_key":  strings.TrimSpace(in.VerifyGroupKey),
			"match_status":      status,
			"match_mode":        "manual",
			"updated_at":        gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "保存导入TG匹配结果失败")
	}
	return s.refreshImportMatchRunCounters(ctx, item.MatchRunId)
}

func (s *sSysPublish) ServerImportRunMatchConfirm(ctx context.Context, in *sysin.ImportRunMatchConfirmInp) error {
	if in == nil || in.ItemId <= 0 {
		return gerror.New("匹配项ID不能为空")
	}
	item, err := s.importRunMatchItem(ctx, in.ItemId)
	if err != nil {
		return err
	}
	if strings.TrimSpace(item.DisplayGroupKey) == "" && strings.TrimSpace(item.VerifyGroupKey) == "" {
		return gerror.New("请先选择展示资料或验证资料消息")
	}
	if err = s.saveImportRunMatchMessages(ctx, item); err != nil {
		return err
	}
	_, err = g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
		Where("id", item.Id).
		Data(g.Map{"match_status": sysin.ImportMatchItemStatusConfirmed, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "确认导入TG匹配失败")
	}
	return s.refreshImportMatchRunCounters(ctx, item.MatchRunId)
}

func (s *sSysPublish) ServerImportRunMatchBatchConfirm(ctx context.Context, in *sysin.ImportRunMatchBatchConfirmInp) error {
	if in == nil || in.MatchRunId <= 0 {
		return gerror.New("匹配运行ID不能为空")
	}
	var items []*sysin.ImportRunMatchItemModel
	err := g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
		Where("match_run_id", in.MatchRunId).
		Where("match_status", sysin.ImportMatchItemStatusAutoSelected).
		WhereNull("deleted_at").
		Scan(&items)
	if err != nil {
		return gerror.Wrap(err, "读取待确认导入TG匹配失败")
	}
	for _, item := range items {
		if item.DisplayGroupKey == "" && item.VerifyGroupKey == "" {
			continue
		}
		if err = s.saveImportRunMatchMessages(ctx, item); err != nil {
			return err
		}
		_, _ = g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
			Where("id", item.Id).
			Data(g.Map{"match_status": sysin.ImportMatchItemStatusConfirmed, "updated_at": gtime.Now()}).
			Update()
	}
	return s.refreshImportMatchRunCounters(ctx, in.MatchRunId)
}

func (s *sSysPublish) ServerImportRunMatchSkip(ctx context.Context, in *sysin.ImportRunMatchSkipInp) error {
	if in == nil || in.ItemId <= 0 {
		return gerror.New("匹配项ID不能为空")
	}
	item, err := s.importRunMatchItem(ctx, in.ItemId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
		Where("id", in.ItemId).
		Data(g.Map{"match_status": sysin.ImportMatchItemStatusSkipped, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "跳过导入TG匹配失败")
	}
	return s.refreshImportMatchRunCounters(ctx, item.MatchRunId)
}

func (s *sSysPublish) ServerImportRunMatchUnbind(ctx context.Context, in *sysin.ImportRunMatchUnbindInp) error {
	if in == nil || in.ItemId <= 0 {
		return gerror.New("匹配项ID不能为空")
	}
	item, err := s.importRunMatchItem(ctx, in.ItemId)
	if err != nil {
		return err
	}
	if err = s.deleteImportMatchBoundMessages(ctx, item); err != nil {
		return err
	}
	_, err = g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
		Where("id", in.ItemId).
		Data(g.Map{
			"display_group_key": "",
			"verify_group_key":  "",
			"match_status":      sysin.ImportMatchItemStatusManualPending,
			"match_mode":        "manual",
			"updated_at":        gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "取消导入TG绑定失败")
	}
	return s.refreshImportMatchRunCounters(ctx, item.MatchRunId)
}

func (s *sSysPublish) ExecuteImportRunMatch(ctx context.Context, matchRunId int64) (err error) {
	run, locked, err := s.lockImportRunMatch(ctx, matchRunId)
	if err != nil || !locked {
		return err
	}
	defer func() {
		if err != nil {
			_ = s.updateImportRunMatch(ctx, matchRunId, g.Map{
				"status":        sysin.ImportMatchRunStatusFailed,
				"stage":         "failed",
				"error_message": err.Error(),
				"finished_at":   gtime.Now(),
				"updated_at":    gtime.Now(),
			})
		}
	}()
	importRun, err := s.importRunMatchImportRun(ctx, run.ImportRunId)
	if err != nil {
		return err
	}
	channelIds := decodeInt64JSON(run.ChannelIdJson)
	channels, err := s.importRunTgMatchChannels(ctx, run.TenantId, channelIds)
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		return gerror.New("未找到可匹配的频道")
	}
	if err = s.updateImportRunMatch(ctx, matchRunId, g.Map{"stage": "candidate", "updated_at": gtime.Now()}); err != nil {
		return err
	}
	candidateTotal, err := s.buildImportMatchCandidatesForChannels(ctx, matchRunId, run.TenantId, channels)
	if err != nil {
		return err
	}
	if err = s.updateImportRunMatch(ctx, matchRunId, g.Map{"stage": "match", "candidate_total": candidateTotal, "updated_at": gtime.Now()}); err != nil {
		return err
	}
	tasks, err := s.importRunMatchTasks(ctx, importRun)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		if err = s.refreshImportMatchRunCounters(ctx, matchRunId); err != nil {
			return err
		}
		return s.updateImportRunMatch(ctx, matchRunId, g.Map{
			"status":      sysin.ImportMatchRunStatusSuccess,
			"stage":       "synced",
			"finished_at": gtime.Now(),
			"updated_at":  gtime.Now(),
		})
	}
	if err = s.updateImportRunMatch(ctx, matchRunId, g.Map{"profile_total": len(tasks), "updated_at": gtime.Now()}); err != nil {
		return err
	}
	profileDone := 0
	for _, taskText := range tasks {
		for _, channel := range channels {
			if err = s.createImportMatchItem(ctx, matchRunId, run.ImportRunId, run.TenantId, run.AccountId, channel.Id, taskText, run.Threshold); err != nil {
				return err
			}
		}
		profileDone++
		_ = s.updateImportRunMatch(ctx, matchRunId, g.Map{"profile_done": profileDone, "updated_at": gtime.Now()})
	}
	if err = s.refreshImportMatchRunCounters(ctx, matchRunId); err != nil {
		return err
	}
	return s.updateImportRunMatch(ctx, matchRunId, g.Map{
		"status":      sysin.ImportMatchRunStatusSuccess,
		"stage":       "finished",
		"finished_at": gtime.Now(),
		"updated_at":  gtime.Now(),
	})
}

func (s *sSysPublish) ExecuteImportRunTgSync(ctx context.Context, matchRunId int64) (err error) {
	run, locked, err := s.lockImportRunMatch(ctx, matchRunId)
	if err != nil || !locked {
		return err
	}
	defer func() {
		if err != nil {
			_ = s.updateImportRunMatch(ctx, matchRunId, g.Map{
				"status":        sysin.ImportMatchRunStatusFailed,
				"stage":         "failed",
				"error_message": err.Error(),
				"finished_at":   gtime.Now(),
				"updated_at":    gtime.Now(),
			})
		}
	}()
	channelIds := decodeInt64JSON(run.ChannelIdJson)
	channels, err := s.importRunTgMatchChannels(ctx, run.TenantId, channelIds)
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		return gerror.New("未找到可同步的频道")
	}
	if err = s.updateImportRunMatch(ctx, matchRunId, g.Map{"stage": "sync", "updated_at": gtime.Now()}); err != nil {
		return err
	}
	scanDays := run.ScanDays
	if scanDays <= 0 {
		scanDays = sysin.DefaultImportTgMatchDays
	}
	cutoff := time.Now().AddDate(0, 0, -scanDays).Unix()
	for _, channel := range channels {
		time.Sleep(400 * time.Millisecond)
		if _, err = s.scanTgChannelMessagesSince(ctx, run.TenantId, channel, cutoff); err != nil {
			return err
		}
	}
	if err = s.updateImportRunMatch(ctx, matchRunId, g.Map{"stage": "candidate", "updated_at": gtime.Now()}); err != nil {
		return err
	}
	candidateTotal, err := s.buildImportMatchCandidatesForChannels(ctx, matchRunId, run.TenantId, channels)
	if err != nil {
		return err
	}
	return s.updateImportRunMatch(ctx, matchRunId, g.Map{
		"status":          sysin.ImportMatchRunStatusSuccess,
		"stage":           "synced",
		"candidate_total": candidateTotal,
		"finished_at":     gtime.Now(),
		"updated_at":      gtime.Now(),
	})
}
func (s *sSysPublish) importRunMatchImportRun(ctx context.Context, importRunId int64) (gdb.Record, error) {
	row, err := g.DB().Model(importRunTable).Safe().Ctx(ctx).
		Where("id", importRunId).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取导入记录失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("导入记录不存在")
	}
	return row, nil
}

func (s *sSysPublish) importRunMatchChannels(ctx context.Context, tenantId int64, channelIds []int64) ([]*sysin.ImportRunMatchChannelModel, error) {
	mod := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,channel_title AS name,target_chat_id,tg_account_id").
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		Where("status", 1).
		WhereNull("deleted_at")
	if len(channelIds) > 0 {
		mod = mod.WhereIn("id", uniqueIds(channelIds))
	}
	var list []*sysin.ImportRunMatchChannelModel
	if err := mod.OrderAsc("id").Scan(&list); err != nil {
		return nil, gerror.Wrap(err, "读取TG匹配频道失败")
	}
	return list, nil
}

func (s *sSysPublish) latestImportRunMatch(ctx context.Context, importRunId int64) (*sysin.ImportRunMatchRunModel, error) {
	var item sysin.ImportRunMatchRunModel
	err := g.DB().Model(publishImportMatchRunTable).Safe().Ctx(ctx).
		Where("import_run_id", importRunId).
		WhereNull("deleted_at").
		OrderDesc("id").
		Limit(1).
		Scan(&item)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, gerror.Wrap(err, "读取最近TG匹配运行失败")
	}
	if item.Id <= 0 {
		return nil, nil
	}
	item.ChannelIds = decodeInt64JSON(item.ChannelIdJson)
	return &item, nil
}

func (s *sSysPublish) lockImportRunMatch(ctx context.Context, matchRunId int64) (sysin.ImportRunMatchRunModel, bool, error) {
	var run sysin.ImportRunMatchRunModel
	result, err := g.DB().Model(publishImportMatchRunTable).Safe().Ctx(ctx).
		Where("id", matchRunId).
		WhereIn("status", []string{sysin.ImportMatchRunStatusPending, sysin.ImportMatchRunStatusFailed}).
		Data(g.Map{"status": sysin.ImportMatchRunStatusRunning, "stage": "running", "error_message": "", "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return run, false, gerror.Wrap(err, "锁定导入TG匹配运行失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return run, false, nil
	}
	if err = g.DB().Model(publishImportMatchRunTable).Safe().Ctx(ctx).Where("id", matchRunId).Scan(&run); err != nil {
		return run, false, gerror.Wrap(err, "读取导入TG匹配运行失败")
	}
	return run, true, nil
}

func (s *sSysPublish) updateImportRunMatch(ctx context.Context, matchRunId int64, data g.Map) error {
	_, err := g.DB().Model(publishImportMatchRunTable).Safe().Ctx(ctx).Where("id", matchRunId).Data(data).Update()
	return err
}

func (s *sSysPublish) importRunMatchTasks(ctx context.Context, importRun gdb.Record) ([]importMatchTaskText, error) {
	var scan sysin.ImportTaskScanModel
	_ = json.Unmarshal([]byte(importRun["result_json"].String()), &scan)
	profileIds := make([]int64, 0)
	for _, item := range scan.Items {
		if item != nil && item.ProfileId > 0 {
			profileIds = append(profileIds, item.ProfileId)
		}
	}
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 {
		return s.importRunMatchFallbackTasks(ctx, importRun)
	}
	return s.importRunMatchTasksByProfileIds(ctx, importRun, profileIds)
}

func (s *sSysPublish) importRunMatchFallbackTasks(ctx context.Context, importRun gdb.Record) ([]importMatchTaskText, error) {
	task, err := s.importTaskRow(ctx, importRun["task_id"].Int64())
	if err != nil {
		return nil, err
	}
	limit := importRun["imported"].Int()
	if limit <= 0 {
		limit = importRun["item_total"].Int()
	}
	if limit <= 0 {
		limit = importRun["recent_count"].Int()
	}
	if limit <= 0 {
		limit = 200
	}
	baseURL := strings.TrimRight(strings.TrimSpace(task["base_url"].String()), "/")
	if baseURL == "" {
		return []importMatchTaskText{}, nil
	}
	like := fmt.Sprintf("legacy:%%:%s:%%", baseURL)
	var rows []struct {
		ProfileId int64 `json:"profileId"`
	}
	profileColumns := dao.ContentProfile.Columns()
	err = g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		Fields("p.id AS profile_id").
		LeftJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Where("ps.tenant_id", importRun["tenant_id"].Int64()).
		Where("ps.account_id", importRun["account_id"].Int64()).
		WhereLike("p."+profileColumns.SourceKey, "youban_publish:"+like).
		Where("p.id >", 0).
		WhereNull("p.deleted_at").
		OrderDesc("p.id").
		Limit(limit).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "按旧站来源读取导入资料失败")
	}
	profileIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		profileIds = append(profileIds, row.ProfileId)
	}
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 {
		return []importMatchTaskText{}, nil
	}
	return s.importRunMatchTasksByProfileIds(ctx, importRun, profileIds)
}

func (s *sSysPublish) importRunMatchTasksByProfileIds(ctx context.Context, importRun gdb.Record, profileIds []int64) ([]importMatchTaskText, error) {
	var rows []struct {
		TaskId    int64  `json:"taskId"`
		ProfileId int64  `json:"profileId"`
		Title     string `json:"title"`
		ProfileNo string `json:"profileNo"`
		SourceKey string `json:"sourceKey"`
		PlainText string `json:"plainText"`
	}
	err := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		Fields("0 AS task_id,p.id AS profile_id,p.title,p.profile_no,p.source_key,p.plain_text").
		LeftJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Where("ps.tenant_id", importRun["tenant_id"].Int64()).
		Where("ps.account_id", importRun["account_id"].Int64()).
		WhereIn("p.id", profileIds).
		WhereNull("p.deleted_at").
		OrderAsc("p.id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取导入资料任务失败")
	}
	tasks := make([]importMatchTaskText, 0, len(rows))
	seen := make(map[int64]struct{})
	for _, row := range rows {
		if _, ok := seen[row.ProfileId]; ok {
			continue
		}
		seen[row.ProfileId] = struct{}{}
		tasks = append(tasks, importMatchTaskText(row))
	}
	return tasks, nil
}

func (s *sSysPublish) buildImportMatchCandidatesForChannels(ctx context.Context, matchRunId int64, tenantId int64, channels []tgMessageRepairChannel) (int, error) {
	candidateTotal := 0
	for _, channel := range channels {
		count, err := s.buildImportMatchCandidates(ctx, matchRunId, tenantId, channel.Id)
		if err != nil {
			return candidateTotal, err
		}
		candidateTotal += count
	}
	return candidateTotal, nil
}

func (s *sSysPublish) buildImportMatchCandidates(ctx context.Context, matchRunId int64, tenantId int64, channelId int64) (int, error) {
	var rows []tgMessageRepairCacheRow
	err := g.DB().Model(publishTgMessageCacheTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("channel_id", channelId).
		Where("media_type <>", "").
		OrderAsc("tg_message_id").
		Scan(&rows)
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG消息缓存失败")
	}
	groups := make(map[string]*importMatchCandidateGroup)
	for _, row := range rows {
		groupKey := importMatchGroupKey(row)
		group := groups[groupKey]
		if group == nil {
			group = &importMatchCandidateGroup{GroupKey: groupKey, MediaGroupId: row.MediaGroupId, MessageDate: row.MessageDate}
			groups[groupKey] = group
		}
		group.MediaCount++
		if group.FirstMessageId == 0 || row.TgMessageId < group.FirstMessageId {
			group.FirstMessageId = row.TgMessageId
		}
		if row.TgMessageId > group.LastMessageId {
			group.LastMessageId = row.TgMessageId
		}
		if strings.TrimSpace(group.CaptionText) == "" && strings.TrimSpace(row.MessageText) != "" {
			group.CaptionText = strings.TrimSpace(row.MessageText)
		}
		group.MediaTypes = mergeCSVValue(group.MediaTypes, row.MediaType)
		if group.MessageDate == nil || (row.MessageDate != nil && row.MessageDate.Time.Before(group.MessageDate.Time)) {
			group.MessageDate = row.MessageDate
		}
	}
	now := gtime.Now()
	count := 0
	for _, group := range groups {
		result, err := g.DB().Model(publishImportMatchCandidateTable).Safe().Ctx(ctx).
			Where("match_run_id", matchRunId).
			Where("channel_id", channelId).
			Where("group_key", group.GroupKey).
			Data(g.Map{
				"tenant_id":        tenantId,
				"media_group_id":   group.MediaGroupId,
				"first_message_id": group.FirstMessageId,
				"last_message_id":  group.LastMessageId,
				"message_date":     group.MessageDate,
				"caption_text":     group.CaptionText,
				"media_count":      group.MediaCount,
				"media_types":      group.MediaTypes,
				"preview_json":     "",
				"updated_at":       now,
			}).
			Update()
		if err != nil {
			return count, gerror.Wrap(err, "更新TG匹配候选失败")
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			_, err = g.DB().Model(publishImportMatchCandidateTable).Safe().Ctx(ctx).Data(g.Map{
				"match_run_id":     matchRunId,
				"tenant_id":        tenantId,
				"channel_id":       channelId,
				"group_key":        group.GroupKey,
				"media_group_id":   group.MediaGroupId,
				"first_message_id": group.FirstMessageId,
				"last_message_id":  group.LastMessageId,
				"message_date":     group.MessageDate,
				"caption_text":     group.CaptionText,
				"media_count":      group.MediaCount,
				"media_types":      group.MediaTypes,
				"preview_json":     "",
				"created_at":       now,
				"updated_at":       now,
			}).Insert()
			if err != nil {
				return count, gerror.Wrap(err, "创建TG匹配候选失败")
			}
		}
		count++
	}
	return count, nil
}

func (s *sSysPublish) createImportMatchItem(ctx context.Context, matchRunId int64, importRunId int64, tenantId int64, accountId int64, channelId int64, taskText importMatchTaskText, threshold int) error {
	candidates, err := s.importMatchChannelCandidates(ctx, matchRunId, channelId)
	if err != nil {
		return err
	}
	var display *sysin.ImportRunMatchCandidateModel
	displayScore := 0
	for _, candidate := range candidates {
		score := importMatchSimilarityScore(taskText, candidate.CaptionText)
		if score > displayScore {
			displayScore = score
			display = candidate
		}
	}
	var verify *sysin.ImportRunMatchCandidateModel
	verifyScore := 0
	if display != nil {
		verify = importMatchNextVerifyCandidate(candidates, display)
		if verify != nil {
			verifyScore = 100
		}
	}
	status := sysin.ImportMatchItemStatusManualPending
	mode := "manual"
	displayKey := ""
	verifyKey := ""
	if display != nil && displayScore >= threshold {
		displayKey = display.GroupKey
		mode = "auto"
		if verify != nil {
			verifyKey = verify.GroupKey
			status = sysin.ImportMatchItemStatusAutoSelected
		}
	}
	reason, _ := json.Marshal(g.Map{"threshold": threshold, "displayScore": displayScore, "verifyScore": verifyScore})
	now := gtime.Now()
	result, err := g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
		Where("match_run_id", matchRunId).
		Where("profile_id", taskText.ProfileId).
		Where("channel_id", channelId).
		Data(g.Map{
			"display_group_key": displayKey,
			"verify_group_key":  verifyKey,
			"display_score":     displayScore,
			"verify_score":      verifyScore,
			"total_score":       displayScore + verifyScore,
			"match_status":      status,
			"match_mode":        mode,
			"reason_json":       string(reason),
			"updated_at":        now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新导入TG匹配项失败")
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		return nil
	}
	_, err = g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).Data(g.Map{
		"match_run_id":      matchRunId,
		"import_run_id":     importRunId,
		"tenant_id":         tenantId,
		"account_id":        accountId,
		"profile_id":        taskText.ProfileId,
		"task_id":           taskText.TaskId,
		"channel_id":        channelId,
		"display_group_key": displayKey,
		"verify_group_key":  verifyKey,
		"display_score":     displayScore,
		"verify_score":      verifyScore,
		"total_score":       displayScore + verifyScore,
		"match_status":      status,
		"match_mode":        mode,
		"reason_json":       string(reason),
		"created_at":        now,
		"updated_at":        now,
	}).Insert()
	if err != nil {
		return gerror.Wrap(err, "创建导入TG匹配项失败")
	}
	return nil
}

func (s *sSysPublish) importMatchChannelCandidates(ctx context.Context, matchRunId int64, channelId int64) ([]*sysin.ImportRunMatchCandidateModel, error) {
	var list []*sysin.ImportRunMatchCandidateModel
	err := g.DB().Model(publishImportMatchCandidateTable).Safe().Ctx(ctx).
		Where("match_run_id", matchRunId).
		Where("channel_id", channelId).
		WhereNull("deleted_at").
		OrderAsc("message_date").
		OrderAsc("first_message_id").
		Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG匹配候选失败")
	}
	return list, nil
}

func (s *sSysPublish) importRunMatchItem(ctx context.Context, itemId int64) (*sysin.ImportRunMatchItemModel, error) {
	var item sysin.ImportRunMatchItemModel
	err := g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
		Where("id", itemId).
		WhereNull("deleted_at").
		Scan(&item)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gerror.New("导入TG匹配项不存在")
		}
		return nil, gerror.Wrap(err, "读取导入TG匹配项失败")
	}
	if item.Id <= 0 {
		return nil, gerror.New("导入TG匹配项不存在")
	}
	return &item, nil
}

func (s *sSysPublish) importMatchTaskText(ctx context.Context, profileId int64) (importMatchTaskText, error) {
	var row importMatchTaskText
	err := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		Fields("0 AS task_id,p.id AS profile_id,p.title,p.profile_no,p.source_key,p.plain_text").
		Where("p.id", profileId).
		WhereNull("p.deleted_at").
		Scan(&row)
	if err != nil {
		return row, gerror.Wrap(err, "读取匹配资料失败")
	}
	if row.ProfileId <= 0 {
		return row, gerror.New("匹配资料不存在")
	}
	return row, nil
}

func (s *sSysPublish) validateImportMatchCandidateKeys(ctx context.Context, item *sysin.ImportRunMatchItemModel, keys ...string) error {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		count, err := g.DB().Model(publishImportMatchCandidateTable).Safe().Ctx(ctx).
			Where("match_run_id", item.MatchRunId).
			Where("channel_id", item.ChannelId).
			Where("group_key", key).
			WhereNull("deleted_at").
			Count()
		if err != nil {
			return gerror.Wrap(err, "校验TG匹配候选失败")
		}
		if count == 0 {
			return gerror.New("选择的TG消息不在当前匹配候选中")
		}
	}
	return nil
}

func (s *sSysPublish) saveImportRunMatchMessages(ctx context.Context, item *sysin.ImportRunMatchItemModel) error {
	task, err := s.tgMessageRepairTask(ctx, item.ProfileId, item.TenantId, item.AccountId)
	if err != nil {
		return err
	}
	if task.IsEmpty() {
		return gerror.New("资料缺少上架任务，无法绑定TG消息")
	}
	channel, err := s.tgRepairChannelById(ctx, item.TenantId, item.ChannelId)
	if err != nil {
		return err
	}
	jobId, err := s.ensureTgRepairJob(ctx, task, channel, channel.TargetChatId)
	if err != nil {
		return err
	}
	if err = s.saveImportMatchGroupMessages(ctx, item, task, jobId, strings.TrimSpace(item.DisplayGroupKey), "display"); err != nil {
		return err
	}
	if err = s.saveImportMatchGroupMessages(ctx, item, task, jobId, strings.TrimSpace(item.VerifyGroupKey), "verify"); err != nil {
		return err
	}
	return nil
}

func (s *sSysPublish) saveImportMatchGroupMessages(ctx context.Context, item *sysin.ImportRunMatchItemModel, task gdb.Record, jobId int64, groupKey string, purpose string) error {
	if groupKey == "" {
		return nil
	}
	rows, err := s.importMatchCacheRowsByGroupKey(ctx, item.TenantId, item.ChannelId, groupKey)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err = s.ensureImportMatchTgMessage(ctx, task, jobId, row, purpose); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) deleteImportMatchBoundMessages(ctx context.Context, item *sysin.ImportRunMatchItemModel) error {
	keys := []string{strings.TrimSpace(item.DisplayGroupKey), strings.TrimSpace(item.VerifyGroupKey)}
	messageIds := make([]int64, 0)
	for _, key := range keys {
		if key == "" {
			continue
		}
		rows, err := s.importMatchCacheRowsByGroupKey(ctx, item.TenantId, item.ChannelId, key)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.TgMessageId > 0 {
				messageIds = append(messageIds, row.TgMessageId)
			}
		}
	}
	messageIds = uniqueIds(messageIds)
	if len(messageIds) == 0 {
		return nil
	}
	_, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("tenant_id", item.TenantId).
		Where("account_id", item.AccountId).
		Where("profile_id", item.ProfileId).
		WhereIn("tg_message_id", messageIds).
		WhereIn("purpose", []string{"display", "verify"}).
		Delete()
	if err != nil {
		return gerror.Wrap(err, "清理导入TG绑定消息失败")
	}
	return nil
}

func (s *sSysPublish) importMatchCacheRowsByGroupKey(ctx context.Context, tenantId int64, channelId int64, groupKey string) ([]tgMessageRepairCacheRow, error) {
	mod := g.DB().Model(publishTgMessageCacheTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("channel_id", channelId).
		Where("media_type <>", "")
	if strings.HasPrefix(groupKey, "group:") {
		mod = mod.Where("media_group_id", strings.TrimPrefix(groupKey, "group:"))
	} else if strings.HasPrefix(groupKey, "msg:") {
		mod = mod.Where("tg_message_id", strings.TrimPrefix(groupKey, "msg:"))
	} else {
		return nil, gerror.New("TG消息组键不合法")
	}
	var rows []tgMessageRepairCacheRow
	if err := mod.OrderAsc("tg_message_id").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取TG消息组缓存失败")
	}
	return rows, nil
}

func (s *sSysPublish) ensureImportMatchTgMessage(ctx context.Context, task gdb.Record, jobId int64, item tgMessageRepairCacheRow, purpose string) error {
	count, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("job_id", jobId).
		Where("tg_message_id", item.TgMessageId).
		Where("status", "sent").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG匹配消息失败")
	}
	if count > 0 {
		_, _ = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Where("job_id", jobId).
			Where("tg_message_id", item.TgMessageId).
			Data(g.Map{"purpose": purpose, "updated_at": gtime.Now()}).
			Update()
		return nil
	}
	var job struct {
		BotId int64 `json:"botId"`
	}
	_ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Fields("bot_id").Where("id", jobId).Scan(&job)
	now := gtime.Now()
	_, err = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":         jobId,
		"task_id":        task["id"].Int64(),
		"tenant_id":      task["tenant_id"].Int64(),
		"account_id":     task["account_id"].Int64(),
		"profile_id":     task["profile_id"].Int64(),
		"bot_id":         job.BotId,
		"target_chat_id": item.TargetChatId,
		"tg_message_id":  item.TgMessageId,
		"media_group_id": item.MediaGroupId,
		"media_id":       0,
		"purpose":        purpose,
		"tg_file_id":     "",
		"status":         "sent",
		"sent_at":        item.MessageDate,
		"created_at":     now,
		"updated_at":     now,
	}).Insert()
	if err != nil {
		return gerror.Wrap(err, "保存导入TG匹配消息失败")
	}
	return nil
}

func (s *sSysPublish) refreshImportMatchRunCounters(ctx context.Context, matchRunId int64) error {
	countStatus := func(status string) int {
		count, _ := g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
			Where("match_run_id", matchRunId).
			Where("match_status", status).
			WhereNull("deleted_at").
			Count()
		return count
	}
	profileTotal, _ := g.DB().Model(publishImportMatchItemTable).Safe().Ctx(ctx).
		Where("match_run_id", matchRunId).
		WhereNull("deleted_at").
		Fields("COUNT(DISTINCT profile_id)").
		Value()
	_, err := g.DB().Model(publishImportMatchRunTable).Safe().Ctx(ctx).Where("id", matchRunId).Data(g.Map{
		"profile_total":  profileTotal.Int(),
		"auto_matched":   countStatus(sysin.ImportMatchItemStatusAutoSelected),
		"manual_pending": countStatus(sysin.ImportMatchItemStatusManualPending),
		"confirmed":      countStatus(sysin.ImportMatchItemStatusConfirmed),
		"skipped":        countStatus(sysin.ImportMatchItemStatusSkipped),
		"updated_at":     gtime.Now(),
	}).Update()
	return err
}

func importMatchGroupKey(row tgMessageRepairCacheRow) string {
	if strings.TrimSpace(row.MediaGroupId) != "" {
		return "group:" + row.MediaGroupId
	}
	return fmt.Sprintf("msg:%d", row.TgMessageId)
}

func mergeCSVValue(csv string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return csv
	}
	parts := strings.Split(csv, ",")
	for _, part := range parts {
		if strings.TrimSpace(part) == value {
			return csv
		}
	}
	if strings.TrimSpace(csv) == "" {
		return value
	}
	return csv + "," + value
}

func importMatchNextVerifyCandidate(candidates []*sysin.ImportRunMatchCandidateModel, display *sysin.ImportRunMatchCandidateModel) *sysin.ImportRunMatchCandidateModel {
	if display == nil || display.MessageDate == nil {
		return nil
	}
	for _, candidate := range candidates {
		if candidate == nil || candidate.GroupKey == display.GroupKey || candidate.MessageDate == nil {
			continue
		}
		if candidate.FirstMessageId <= display.LastMessageId {
			continue
		}
		if candidate.MessageDate.Time.Sub(display.MessageDate.Time) > 10*time.Minute {
			continue
		}
		if strings.Contains(candidate.MediaTypes, "video") || candidate.MediaCount > 0 {
			return candidate
		}
	}
	return nil
}

func importMatchSimilarityScore(task importMatchTaskText, caption string) int {
	captionNorm := normalizeImportMatchText(caption)
	if captionNorm == "" {
		return 0
	}
	title := normalizeImportMatchText(task.Title)
	if title != "" && strings.Contains(captionNorm, title) {
		return 100
	}
	profileNo := normalizeImportMatchText(task.ProfileNo)
	if profileNo != "" && strings.Contains(captionNorm, profileNo) {
		return 95
	}
	sourceKey := normalizeImportMatchText(task.SourceKey)
	if sourceKey != "" && strings.Contains(captionNorm, sourceKey) {
		return 90
	}
	source := normalizeImportMatchText(strings.Join([]string{task.Title, task.ProfileNo, task.SourceKey, task.PlainText}, " "))
	if source == "" {
		return 0
	}
	return bigramSimilarity(source, captionNorm)
}

func normalizeImportMatchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`https?://\S+`).ReplaceAllString(value, "")
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func bigramSimilarity(a string, b string) int {
	if a == "" || b == "" {
		return 0
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return 85
	}
	gramsA := bigrams(a)
	gramsB := bigrams(b)
	if len(gramsA) == 0 || len(gramsB) == 0 {
		return 0
	}
	intersection := 0
	for key, countA := range gramsA {
		if countB, ok := gramsB[key]; ok {
			if countA < countB {
				intersection += countA
			} else {
				intersection += countB
			}
		}
	}
	return intersection * 200 / (sumGramCount(gramsA) + sumGramCount(gramsB))
}

func bigrams(value string) map[string]int {
	runes := []rune(value)
	res := make(map[string]int)
	if len(runes) == 1 {
		res[string(runes)] = 1
		return res
	}
	for i := 0; i < len(runes)-1; i++ {
		res[string(runes[i:i+2])]++
	}
	return res
}

func sumGramCount(values map[string]int) int {
	total := 0
	for _, count := range values {
		total += count
	}
	return total
}
