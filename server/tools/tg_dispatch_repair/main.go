package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"

	publishsys "hotgo/addons/youban_publish/logic/sys"
	"hotgo/internal/bootstrap/envconfig"
	"hotgo/internal/library/cache"
	_ "hotgo/internal/logic/common"
	_ "hotgo/internal/logic/sys"
	"hotgo/internal/service"
)

func main() {
	limit := flag.Int("limit", 100, "maximum jobs to schedule")
	jobID := flag.Int64("job-id", 0, "schedule one existing Telegram job")
	collect := flag.Bool("collect", false, "recover stalled collection events")
	flag.Parse()
	ctx := context.Background()
	envconfig.Apply(ctx)
	cache.SetAdapter(ctx)
	service.SysConfig().InitConfig(ctx)
	publisher := publishsys.NewSysPublish()
	var err error
	if *collect {
		err = publisher.RecoverCollectEvents(ctx, *limit)
	} else if *jobID > 0 {
		err = publisher.RecoverTelegramJobDispatch(ctx, *jobID)
	} else {
		err = publisher.RecoverTelegramDispatch(ctx, *limit)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("telegram dispatch recovery completed, job=%d limit=%d\n", *jobID, *limit)
}
