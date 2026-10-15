package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	publishsys "hotgo/addons/youban_publish/logic/sys"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type mediaAudit struct {
	ID       int64  `json:"id"`
	SHA256   string `json:"sha256,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
	Readable bool   `json:"readable"`
	Reason   string `json:"reason,omitempty"`
}
type pairAudit struct {
	publishsys.CollectRepairCandidate
	Media []mediaAudit `json:"media"`
	Ready bool         `json:"ready"`
}

type replayLane struct {
	key     string
	indexes []int
}

func buildReplayLanes(pairs []publishsys.CollectRepairCandidate) []replayLane {
	lanes := make([]replayLane, 0)
	positions := make(map[string]int)
	for i, pair := range pairs {
		key := fmt.Sprintf("%d:%d", pair.TenantID, pair.AccountID)
		position, ok := positions[key]
		if !ok {
			position = len(lanes)
			positions[key] = position
			lanes = append(lanes, replayLane{key: key})
		}
		lanes[position].indexes = append(lanes[position].indexes, i)
	}
	return lanes
}

func auditMedia(ctx context.Context, m publishsys.CollectRepairMedia) mediaAudit {
	out := mediaAudit{ID: m.ID}
	if m.Status != "ready" || m.URL == "" {
		out.Reason = "media_not_cached"
		return out
	}
	probeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, "GET", m.URL, nil)
	if err != nil {
		out.Reason = "invalid_media_url"
		return out
	}
	// 应用初始化可能替换 DefaultClient；校验使用独立客户端并保留环境代理。
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 60 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		out.Reason = "media_download_failed"
		return out
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		out.Reason = fmt.Sprintf("http_%d", res.StatusCode)
		return out
	}
	file, err := os.CreateTemp("", "collect-repair-probe-*")
	if err != nil {
		out.Reason = "temp_file_failed"
		return out
	}
	defer os.Remove(file.Name())
	hash := sha256.New()
	out.Bytes, err = io.Copy(io.MultiWriter(file, hash), io.LimitReader(res.Body, 512*1024*1024+1))
	file.Close()
	if err != nil || out.Bytes == 0 || out.Bytes > 512*1024*1024 {
		out.Reason = "invalid_media_size"
		return out
	}
	out.SHA256 = hex.EncodeToString(hash.Sum(nil))
	cmd := exec.CommandContext(probeCtx, "ffmpeg", "-nostdin", "-v", "error", "-xerror", "-i", file.Name(), "-map", "0:v:0", "-f", "null", "-")
	if err := cmd.Run(); err != nil {
		out.Reason = "media_decode_failed"
		return out
	}
	out.Readable = true
	return out
}

func runAudit(ctx context.Context, opts options) error {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}
	since, err := time.ParseInLocation("2006-01-02", opts.date, location)
	if err != nil {
		return fmt.Errorf("date must be YYYY-MM-DD")
	}
	pairs, unresolved, err := publishsys.ScanSuspendedCollectRepair(ctx, opts.tenantID, opts.accountID, since, since.AddDate(0, 0, 1), opts.limit)
	if err != nil {
		return err
	}
	if opts.eventIDs != "" {
		ids, err := parseEventIDs(opts.eventIDs)
		if err != nil {
			return err
		}
		selected := map[int64]bool{}
		for _, id := range ids {
			selected[id] = true
		}
		filtered := make([]publishsys.CollectRepairCandidate, 0, len(ids))
		for _, pair := range pairs {
			if selected[pair.DisplayID] {
				filtered = append(filtered, pair)
			}
		}
		if len(filtered) != len(ids) {
			return fmt.Errorf("audit event IDs must be display candidates in the selected scope")
		}
		pairs = filtered
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].DisplayID < pairs[j].DisplayID })
	report := struct {
		Date       string      `json:"date"`
		TenantID   int64       `json:"tenantId"`
		AccountID  int64       `json:"accountId"`
		Pairs      []pairAudit `json:"pairs"`
		Unresolved []int64     `json:"unresolved"`
	}{Date: opts.date, TenantID: opts.tenantID, AccountID: opts.accountID, Unresolved: unresolved}
	for _, pair := range pairs {
		fmt.Printf("checking display %d verify %d\n", pair.DisplayID, pair.VerifyID)
		audit := pairAudit{CollectRepairCandidate: pair, Ready: true}
		hasVideo := false
		for _, eventID := range []int64{pair.DisplayID, pair.VerifyID} {
			media, prepareErr := publishsys.PrepareSuspendedCollectRepair(ctx, eventID, opts.download)
			if prepareErr != nil {
				audit.Ready = false
				fmt.Fprintf(os.Stderr, "event %d: prepare failed; remains suspended\n", eventID)
				continue
			}
			for _, m := range media {
				check := auditMedia(ctx, m)
				if eventID == pair.VerifyID && m.Kind == "video" && check.Readable {
					hasVideo = true
				}
				audit.Media = append(audit.Media, check)
				if !check.Readable {
					audit.Ready = false
				}
			}
		}
		audit.Ready = audit.Ready && hasVideo
		report.Pairs = append(report.Pairs, audit)
	}
	file, err := os.OpenFile(opts.report, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(report); err != nil {
		return err
	}
	fmt.Printf("audit: %d candidate pairs, %d unresolved events; report %s; all events remain suspended\n", len(pairs), len(unresolved), opts.report)
	return nil
}

func runApproved(ctx context.Context, opts options) error {
	if opts.replayConcurrency < 1 || opts.replayConcurrency > 32 {
		return fmt.Errorf("replay concurrency must be between 1 and 32")
	}
	if opts.concurrency < 1 || opts.concurrency > 32 {
		return fmt.Errorf("concurrency must be between 1 and 32")
	}
	var since, until time.Time
	tenantID, accountID := opts.tenantID, opts.accountID
	if opts.allSuspended {
		tenantID, accountID = 0, 0
	} else {
		location, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			return err
		}
		var errDate error
		since, errDate = time.ParseInLocation("2006-01-02", opts.date, location)
		if errDate != nil {
			return errDate
		}
		until = since.AddDate(0, 0, 1)
	}
	scanCtx, scanCancel := context.WithTimeout(ctx, 2*time.Minute)
	candidates, unresolved, err := publishsys.ScanSuspendedCollectRepair(scanCtx, tenantID, accountID, since, until, opts.limit, opts.sourceID)
	scanCancel()
	if err != nil {
		return err
	}
	pairs := candidates
	if opts.sourceID > 0 {
		pairs = nil
		for _, pair := range candidates {
			if pair.SourceID == opts.sourceID {
				pairs = append(pairs, pair)
			}
		}
	}
	if opts.eventIDs != "" {
		ids, err := parseEventIDs(opts.eventIDs)
		if err != nil {
			return err
		}
		selected := map[int64]bool{}
		for _, id := range ids {
			selected[id] = true
		}
		scopedPairs := pairs
		pairs = nil
		for _, pair := range scopedPairs {
			if selected[pair.DisplayID] {
				pairs = append(pairs, pair)
			}
		}
		if len(pairs) != len(ids) {
			return fmt.Errorf("event IDs must be display candidates in selected scope")
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		a, b := pairs[i], pairs[j]
		if a.TenantID != b.TenantID {
			return a.TenantID < b.TenantID
		}
		if a.AccountID != b.AccountID {
			return a.AccountID < b.AccountID
		}
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		if a.ChatID != b.ChatID {
			return a.ChatID < b.ChatID
		}
		if a.MessageID != b.MessageID {
			return a.MessageID < b.MessageID
		}
		return a.DisplayID < b.DisplayID
	})
	fmt.Printf("repair scan: pairs=%d unresolved=%d workers=%d\n", len(pairs), len(unresolved), opts.concurrency)
	report := struct {
		Pairs      []pairAudit `json:"pairs"`
		Unresolved []int64     `json:"unresolved"`
	}{Pairs: make([]pairAudit, len(pairs)), Unresolved: unresolved}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make([]chan struct{}, len(pairs))
	for i := range done {
		done[i] = make(chan struct{})
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for w := 0; w < opts.concurrency; w++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				pair := pairs[i]
				fmt.Printf("checking [%d/%d] display=%d verify=%d\n", i+1, len(pairs), pair.DisplayID, pair.VerifyID)
				out := pairAudit{CollectRepairCandidate: pair, Ready: true}
				prepareCtx, prepareCancel := context.WithTimeout(workCtx, 3*time.Minute)
				hasVideo := false
				for _, id := range []int64{pair.DisplayID, pair.VerifyID} {
					media, err := publishsys.PrepareSuspendedCollectRepair(prepareCtx, id, true)
					if err != nil {
						out.Ready = false
						out.Media = append(out.Media, mediaAudit{Reason: err.Error()})
						break
					}
					for _, m := range media {
						check := auditMedia(prepareCtx, m)
						out.Media = append(out.Media, check)
						if !check.Readable {
							out.Ready = false
						}
						if id == pair.VerifyID && m.Kind == "video" && check.Readable {
							hasVideo = true
						}
					}
				}
				prepareCancel()
				out.Ready = out.Ready && hasVideo
				report.Pairs[i] = out
				fmt.Printf("checking completed [%d/%d] display=%d ready=%t media=%d\n", i+1, len(pairs), pair.DisplayID, out.Ready, len(out.Media))
				close(done[i])
			}
		}()
	}
	feederDone := make(chan struct{})
	go func() {
		defer close(feederDone)
		defer close(jobs)
		for i := range pairs {
			select {
			case jobs <- i:
			case <-workCtx.Done():
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-feederDone
		workers.Wait()
		data, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			if err = os.WriteFile(opts.report, data, 0600); err != nil {
				fmt.Fprintf(os.Stderr, "repair report failed: %v\n", err)
			}
		}
	}()
	lanes := buildReplayLanes(pairs)
	replayWorkers := opts.replayConcurrency
	if replayWorkers > len(lanes) {
		replayWorkers = len(lanes)
	}
	fmt.Printf("repair replay: lanes=%d workers=%d\n", len(lanes), replayWorkers)
	laneJobs := make(chan replayLane)
	var (
		replayWG     sync.WaitGroup
		failed       atomic.Int64
		replayErrors = make(chan error, len(lanes))
	)
	for w := 0; w < replayWorkers; w++ {
		replayWG.Add(1)
		go func() {
			defer replayWG.Done()
			for lane := range laneJobs {
				for _, i := range lane.indexes {
					pair := pairs[i]
					select {
					case <-done[i]:
					case <-ctx.Done():
						replayErrors <- ctx.Err()
						return
					}
					if !report.Pairs[i].Ready {
						failed.Add(1)
						fmt.Printf("held display %d: incomplete or unreadable media; continuing later complete groups\n", pair.DisplayID)
						continue
					}
					fmt.Printf("checking replay [%d/%d] lane=%s display=%d verify=%d; waiting for dispatch\n", i+1, len(pairs), lane.key, pair.DisplayID, pair.VerifyID)
					if err := publishsys.ReleaseSuspendedCollectRepair(ctx, pair.TenantID, pair.AccountID, pair); err != nil {
						replayErrors <- fmt.Errorf("replay failed display %d: %w", pair.DisplayID, err)
						break
					}
					if err := publishsys.WaitCollectRepairDispatch(ctx, pair.DisplayID); err != nil {
						replayErrors <- fmt.Errorf("replay dispatch failed display %d: %w", pair.DisplayID, err)
						break
					}
					fmt.Printf("released and replayed display %d verify %d lane=%s; dispatch settled (may be sent, ignored, skipped or reviewing)\n", pair.DisplayID, pair.VerifyID, lane.key)
				}
			}
		}()
	}
	for _, lane := range lanes {
		select {
		case laneJobs <- lane:
		case <-ctx.Done():
			close(laneJobs)
			replayWG.Wait()
			return ctx.Err()
		}
	}
	close(laneJobs)
	replayWG.Wait()
	close(replayErrors)
	for replayErr := range replayErrors {
		return replayErr
	}
	if failed.Load() > 0 {
		return fmt.Errorf("repair batch has %d held pairs; report: %s", failed.Load(), opts.report)
	}
	return nil
}
