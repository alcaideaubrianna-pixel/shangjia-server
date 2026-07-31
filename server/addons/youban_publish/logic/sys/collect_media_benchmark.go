package sys

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CollectMediaBenchmark(ctx context.Context, in *sysin.CollectMediaBenchmarkInp) (*sysin.CollectMediaBenchmarkModel, error) {
	var account *sysin.AccountModel
	var err error
	if in != nil && in.AccountId > 0 && s.isSystemSuperAdmin(ctx) {
		account, err = s.benchmarkAccountById(ctx, in.AccountId)
	} else {
		account, err = s.currentAccount(ctx)
	}
	if err != nil {
		return nil, err
	}
	limit, concurrency := 30, 4
	includeCached := false
	if in != nil {
		if in.Limit > 0 {
			limit = in.Limit
		}
		if in.Concurrency > 0 {
			concurrency = in.Concurrency
		}
		includeCached = in.IncludeCached
	}
	if limit > 200 {
		limit = 200
	}
	if concurrency > 32 {
		concurrency = 32
	}

	model := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		As("m").
		LeftJoin(pdao.YoubanPublishCollectEvent.Table()+" e", "e.id=m.event_id").
		Fields("m.*", "e.tg_account_id").
		Where("m.tenant_id", account.TenantId).
		Where("m.account_id", account.Id).
		WhereLike("m.source_file_id", "gotd:%").
		Where("e.tg_account_id >", 0)
	if in != nil && in.SourceId > 0 {
		model = model.Where("m.source_id", in.SourceId)
	}
	if !includeCached {
		model = model.Where("m.storage_path", "").Where("m.file_url", "")
	}
	rows, err := model.OrderAsc("m.id").Limit(limit).All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG媒体压测样本失败")
	}
	result := &sysin.CollectMediaBenchmarkModel{
		Total:       len(rows),
		Concurrency: concurrency,
		Items:       make([]*sysin.CollectMediaBenchmarkItem, len(rows)),
	}
	if len(rows) == 0 {
		return result, nil
	}

	startedAt := time.Now()
	jobs := make(chan int)
	var wg sync.WaitGroup
	if concurrency > len(rows) {
		concurrency = len(rows)
		result.Concurrency = concurrency
	}
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				row := rows[index]
				item := &sysin.CollectMediaBenchmarkItem{
					MediaId:     row["id"].Int64(),
					EventId:     row["event_id"].Int64(),
					SourceId:    row["source_id"].Int64(),
					TgAccountId: row["tg_account_id"].Int64(),
					MediaType:   row["media_type"].String(),
				}
				item.ExpectedSize = row["source_size"].Int64()
				start := time.Now()
				downloaded, downloadErr := s.downloadTelegramMedia(ctx, account.TenantId, item.TgAccountId, collectMediaItem{
					Type: row["media_type"].String(), FileId: row["source_file_id"].String(),
					FileUrl: row["file_url"].String(), StoragePath: row["storage_path"].String(), PosterUrl: row["poster_url"].String(),
					SourceKind: row["source_kind"].String(), SourceMediaId: row["source_media_id"].Int64(),
					SourceAccessHash: row["source_access_hash"].Int64(), SourceFileReference: row["source_file_reference"].Bytes(),
					SourceThumbSize: row["source_thumb_size"].String(), SourceMimeType: row["source_mime_type"].String(),
					SourceDCId: row["source_dc_id"].Int(), SourceSize: row["source_size"].Int64(),
				})
				item.DurationMs = time.Since(start).Milliseconds()
				item.Success = downloadErr == nil
				if downloaded != nil && item.ExpectedSize <= 0 {
					item.ExpectedSize, _ = fileSize(downloaded.Path)
				}
				if downloadErr != nil {
					item.Error = downloadErr.Error()
				}
				result.Items[index] = item
			}
		}()
	}
	for index := range rows {
		result.Started++
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	result.TotalDurationMs = time.Since(startedAt).Milliseconds()

	durations := make([]int64, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		result.Bytes += item.ExpectedSize
		if item.Success {
			result.Succeeded++
		} else {
			result.Failed++
		}
		if item.DurationMs > 0 {
			durations = append(durations, item.DurationMs)
		}
	}
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		result.P50DurationMs = percentileDuration(durations, 0.50)
		result.P95DurationMs = percentileDuration(durations, 0.95)
	}
	if result.TotalDurationMs > 0 {
		result.ThroughputMbps = float64(result.Bytes) * 8 / float64(result.TotalDurationMs) / 1000
	}
	return result, nil
}

func (s *sSysPublish) benchmarkAccountById(ctx context.Context, accountId int64) (*sysin.AccountModel, error) {
	if accountId <= 0 {
		return nil, gerror.New("上架账号ID不能为空")
	}
	var account *sysin.AccountModel
	cols := pdao.YoubanPublishAccount.Columns()
	err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(cols.Id, accountId).
		Where(cols.Status, 1).
		WhereNull(cols.DeletedAt).
		Scan(&account)
	if err != nil {
		return nil, gerror.Wrap(err, "读取压测上架账号失败")
	}
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("压测上架账号不存在或已停用")
	}
	return account, nil
}

func percentileDuration(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * percentile)
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}
