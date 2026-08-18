package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/dao"
	"hotgo/internal/library/profileextractor"
)

type options struct {
	batchSize int
	startID   int64
	dryRun    bool
}

func main() {
	opts := options{}
	flag.IntVar(&opts.batchSize, "batch-size", 200, "number of profiles processed per batch")
	flag.Int64Var(&opts.startID, "start-id", 0, "resume after this profile ID")
	flag.BoolVar(&opts.dryRun, "dry-run", true, "show changes without writing them")
	flag.Parse()
	if opts.batchSize < 1 || opts.batchSize > 2000 {
		fmt.Fprintln(os.Stderr, "batch-size must be between 1 and 2000")
		os.Exit(2)
	}
	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options) error {
	columns := dao.ContentProfile.Columns()
	lastID := opts.startID
	totalScanned := 0
	totalUpdated := 0
	for {
		rows, err := dao.ContentProfile.Ctx(ctx).
			WhereGT(columns.Id, lastID).
			Where("("+columns.Height+" = 0 OR "+columns.Weight+" = 0 OR "+columns.CupSize+" = '')").
			Fields(columns.Id, columns.PlainText, columns.Height, columns.Weight, columns.CupSize).
			OrderAsc(columns.Id).
			Limit(opts.batchSize).
			All()
		if err != nil {
			return fmt.Errorf("query profiles after id %d: %w", lastID, err)
		}
		if rows.IsEmpty() {
			break
		}
		for _, row := range rows {
			id := row[columns.Id].Int64()
			lastID = id
			totalScanned++
			fields := profileextractor.Merge(
				row[columns.PlainText].String(),
				row[columns.Height].Int(),
				row[columns.Weight].Int(),
				row[columns.CupSize].String(),
			)
			data := g.Map{}
			if row[columns.Height].Int() == 0 && fields.Height > 0 {
				data[columns.Height] = fields.Height
			}
			if row[columns.Weight].Int() == 0 && fields.Weight > 0 {
				data[columns.Weight] = fields.Weight
			}
			if row[columns.CupSize].String() == "" && fields.Cup != "" {
				data[columns.CupSize] = fields.Cup
			}
			if len(data) == 0 {
				continue
			}
			totalUpdated++
			fmt.Printf("profile=%d changes=%v dryRun=%t\n", id, data, opts.dryRun)
			if !opts.dryRun {
				if _, err = dao.ContentProfile.Ctx(ctx).Where(columns.Id, id).Data(data).Update(); err != nil {
					return fmt.Errorf("update profile %d: %w", id, err)
				}
			}
		}
		fmt.Printf("progress lastId=%d scanned=%d changed=%d\n", lastID, totalScanned, totalUpdated)
	}
	fmt.Printf("done lastId=%d scanned=%d changed=%d dryRun=%t\n", lastID, totalScanned, totalUpdated, opts.dryRun)
	return nil
}
