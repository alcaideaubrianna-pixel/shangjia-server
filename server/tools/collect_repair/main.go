package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"

	_ "hotgo/addons/telegram_collector/logic/sys"
	_ "hotgo/addons/youban_bot/logic/sys"
	publishsys "hotgo/addons/youban_publish/logic/sys"
	_ "hotgo/addons/youban_tg_bot_gateway/logic/sys"
	"hotgo/internal/bootstrap/envconfig"
	"hotgo/internal/library/cache"
	_ "hotgo/internal/logic/common"
	_ "hotgo/internal/logic/sys"
	"hotgo/internal/service"
)

type options struct {
	profileIDs           string
	sourceID             int64
	allSuspended         bool
	concurrency          int
	replayConcurrency    int
	apply                bool
	audit                bool
	download             bool
	date                 string
	tenantID             int64
	accountID            int64
	limit                int
	report               string
	sshHost              string
	sshIdentity          string
	dbPort               int
	redisPort            int
	eventIDs             string
	verifyOnly           bool
	dryRun               bool
	continueOnError      bool
	misbindingAudit      bool
	misbindingApply      bool
	misbindingRepublish  bool
	misbindingSince      string
	misbindingUntil      string
	cleanupWait          time.Duration
	recoverIgnoredVerify bool
}

func main() { os.Exit(mainExit()) }

func mainExit() int {
	var opts options
	flag.StringVar(&opts.profileIDs, "profile-ids", "", "rebuild failed BOT profile media in current storage without publishing")
	flag.StringVar(&opts.eventIDs, "event-ids", "", "comma-separated display collect event IDs")
	flag.BoolVar(&opts.verifyOnly, "verify-only", false, "queue verification only for displays already sent to Telegram")
	flag.BoolVar(&opts.dryRun, "dry-run", true, "validate and print target event IDs without modifying data")
	flag.BoolVar(&opts.continueOnError, "continue-on-error", true, "continue processing later events when one event fails")
	flag.BoolVar(&opts.audit, "audit", false, "scan suspended events and decode cached media; does not publish")
	flag.BoolVar(&opts.download, "download", false, "download pending media during audit, retaining suspension")
	flag.StringVar(&opts.date, "date", "2026-10-12", "Shanghai creation date to repair")
	flag.Int64Var(&opts.tenantID, "tenant-id", 76, "explicit tenant scope")
	flag.Int64Var(&opts.accountID, "account-id", 613, "explicit owner account scope")
	flag.IntVar(&opts.limit, "limit", 100, "maximum suspended events; fail if exceeded")
	flag.StringVar(&opts.report, "report", "collect-repair-report.json", "audit report (contains IDs and hashes only)")
	flag.StringVar(&opts.sshHost, "ssh-host", "", "SSH bastion user@host; enables managed DB/Redis tunnel")
	flag.StringVar(&opts.sshIdentity, "ssh-identity", "", "existing SSH identity path")
	flag.IntVar(&opts.dbPort, "db-port", 15432, "local DB tunnel port")
	flag.IntVar(&opts.redisPort, "redis-port", 16379, "local Redis tunnel port")
	flag.Int64Var(&opts.sourceID, "source-id", 0, "restrict suspended repair to one collect source")
	flag.BoolVar(&opts.allSuspended, "all-suspended", false, "repair all suspended events across tenants, accounts and dates")
	flag.IntVar(&opts.concurrency, "concurrency", 8, "parallel media preparation workers (1-32); publishing remains ordered")
	flag.IntVar(&opts.replayConcurrency, "replay-concurrency", 1, "parallel account replay lanes (1-32); each account remains ordered")
	flag.BoolVar(&opts.apply, "apply", false, "decode all media again, then release complete suspended pairs and replay")
	flag.BoolVar(&opts.misbindingAudit, "misbinding-audit", false, "audit historical verification-video misbindings; never changes data")
	flag.BoolVar(&opts.misbindingApply, "misbinding-apply", false, "apply only unambiguous historical verification-video repairs")
	flag.BoolVar(&opts.misbindingRepublish, "misbinding-republish", false, "after repair, wait for cleanup and republish to the exact original channels")
	flag.StringVar(&opts.misbindingSince, "misbinding-since", "2026-10-14 00:00:00", "Shanghai start time for historical misbinding scan")
	flag.StringVar(&opts.misbindingUntil, "misbinding-until", "2026-10-16 00:00:00", "Shanghai exclusive end time for historical misbinding scan")
	flag.DurationVar(&opts.cleanupWait, "cleanup-wait", 30*time.Minute, "maximum total wait for Telegram cleanup before republishing")
	flag.BoolVar(&opts.recoverIgnoredVerify, "recover-ignored-verify", false, "canonically pair and enqueue ignored verification videos")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.sshHost != "" {
		cleanup, err := startTunnel(ctx, opts.sshHost, opts.sshIdentity, opts.dbPort, opts.redisPort)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer cleanup()
		os.Setenv("YOUBAN_DB_HOST", "127.0.0.1")
		os.Setenv("YOUBAN_DB_PORT", strconv.Itoa(opts.dbPort))
		os.Setenv("YOUBAN_REDIS_ADDRESS", fmt.Sprintf("127.0.0.1:%d", opts.redisPort))
	}

	if endpoint := os.Getenv("COLLECT_REPAIR_BOT_API_URL"); endpoint != "" {
		os.Setenv("YOUBAN_TELEGRAM_BOT_API_URL", endpoint)
		os.Setenv("YOUBAN_TELEGRAM_BOT_FILE_URL", os.Getenv("COLLECT_REPAIR_BOT_FILE_URL"))
	}
	if err := run(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func run(ctx context.Context, opts options) error {
	if opts.recoverIgnoredVerify {
		envconfig.Apply(ctx)
		cache.SetAdapter(ctx)
		service.SysConfig().InitConfig(ctx)
		processed, failed, err := publishsys.NewSysPublish().AIOpsRecoverIgnoredCollectVerifications(ctx, opts.sourceID, opts.limit, opts.dryRun)
		if err != nil {
			return err
		}
		fmt.Printf("ignored verification recovery: processed=%d failed=%d dry-run=%v failedIds=%v\n", processed, len(failed), opts.dryRun, failed)
		return nil
	}
	if opts.misbindingAudit || opts.misbindingApply {
		if opts.misbindingRepublish && !opts.misbindingApply {
			return fmt.Errorf("misbinding-republish requires misbinding-apply")
		}
		envconfig.Apply(ctx)
		cache.SetAdapter(ctx)
		service.SysConfig().InitConfig(ctx)
		return runMisbindingRepair(ctx, opts)
	}
	if opts.profileIDs != "" {
		ids, err := parseEventIDs(opts.profileIDs)
		if err != nil {
			return err
		}
		envconfig.Apply(ctx)
		cache.SetAdapter(ctx)
		service.SysConfig().InitConfig(ctx)
		for _, id := range ids {
			fmt.Printf("checking profile media %d dry-run=%v\n", id, opts.dryRun)
			result, err := publishsys.NewSysPublish().AIOpsRebuildProfileMedia(ctx, []int64{id}, opts.dryRun)
			if err != nil {
				return fmt.Errorf("profile %d failed: %w", id, err)
			}
			fmt.Printf("checking profile media result: %+v\n", result)
		}
		return nil
	}
	if opts.apply {
		fmt.Println("checking initialization: configuration")
		envconfig.Apply(ctx)
		fmt.Println("checking initialization: cache")
		cache.SetAdapter(ctx)
		fmt.Println("checking initialization: system configuration")
		service.SysConfig().InitConfig(ctx)
		fmt.Println("checking initialization: scanning suspended events")
		return runApproved(ctx, opts)
	}
	if opts.verifyOnly {
		eventIDs, err := parseEventIDs(opts.eventIDs)
		if err != nil {
			return err
		}
		envconfig.Apply(ctx)
		cache.SetAdapter(ctx)
		service.SysConfig().InitConfig(ctx)
		processed, err := publishsys.NewSysPublish().AIOpsQueueCollectVerification(ctx, eventIDs, opts.dryRun)
		if err != nil {
			return fmt.Errorf("queue collect verification: %w", err)
		}
		fmt.Printf("verification-only repair complete: checked %d event(s), dry-run=%v\n", processed, opts.dryRun)
		return nil
	}
	if opts.audit {
		envconfig.Apply(ctx)
		if opts.download {
			cache.SetAdapter(ctx)
			service.SysConfig().InitConfig(ctx)
		}
		return runAudit(ctx, opts)
	}
	eventIDs, err := parseEventIDs(opts.eventIDs)
	if err != nil {
		return err
	}
	if opts.dryRun {
		fmt.Printf("dry-run: validated %d event ID(s): %v; no database actions\n", len(eventIDs), eventIDs)
		return nil
	}
	envconfig.Apply(ctx)
	cache.SetAdapter(ctx)
	service.SysConfig().InitConfig(ctx)

	if opts.continueOnError {
		processed, failed, err := publishsys.RequeueCollectEventsBestEffort(ctx, eventIDs)
		if err != nil {
			return fmt.Errorf("repair collect events: %w", err)
		}
		fmt.Printf("repair complete: processed %d, failed %d\n", processed, len(failed))
		if len(failed) > 0 {
			fmt.Printf("failed event IDs: %v\n", failed)
		}
		return nil
	}
	processed, err := publishsys.RequeueCollectEvents(ctx, eventIDs)
	if err != nil {
		return fmt.Errorf("repair collect events: %w", err)
	}
	if processed != len(eventIDs) {
		return fmt.Errorf("repair incomplete: processed %d of %d events", processed, len(eventIDs))
	}
	fmt.Printf("repair complete: processed %d event(s): %v\n", processed, eventIDs)
	return nil
}

func parseEventIDs(value string) ([]int64, error) {
	parts := strings.Split(value, ",")
	ids := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid event ID %q", part)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("event-ids is required")
	}
	return ids, nil
}
