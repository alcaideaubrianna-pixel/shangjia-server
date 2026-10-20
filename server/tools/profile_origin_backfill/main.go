package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	publishsys "hotgo/addons/youban_publish/logic/sys"
	"hotgo/internal/bootstrap/envconfig"
	"os"
	"time"
)

func main() {
	tenant := flag.Int64("tenant-id", 0, "required tenant scope")
	account := flag.Int64("account-id", 0, "required account scope")
	after := flag.Int64("after-id", 0, "exclusive profile cursor")
	limit := flag.Int("limit", 100, "batch size, 1..500")
	apply := flag.Bool("apply", false, "persist origins; default read-only preview")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	envconfig.Apply(ctx)
	result, err := publishsys.NewSysPublish().BackfillProfileOrigins(ctx, *tenant, *account, *after, *limit, *apply)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
