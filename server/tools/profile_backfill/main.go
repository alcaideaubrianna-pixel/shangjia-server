package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/gogf/gf/contrib/drivers/mysql/v2"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/dao"
	"hotgo/internal/library/profileextractor"
)

type options struct {
	batchSize   int
	startID     int64
	dryRun      bool
	strict      bool
	maxBatches  int
	indexesOnly bool
}

func main() {
	opts := options{}
	flag.IntVar(&opts.batchSize, "batch-size", 1000, "number of profiles processed per batch")
	flag.Int64Var(&opts.startID, "start-id", 0, "resume after this profile ID")
	flag.BoolVar(&opts.dryRun, "dry-run", true, "show changes without writing them")
	flag.BoolVar(&opts.strict, "strict", true, "stop before writing a batch containing anomalous profile fields")
	flag.IntVar(&opts.maxBatches, "max-batches", 0, "stop after this many batches; zero processes all batches")
	flag.BoolVar(&opts.indexesOnly, "indexes-only", false, "create profile field indexes and exit")
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
	if opts.indexesOnly {
		return ensureProfileFieldIndexes(ctx)
	}
	columns := dao.ContentProfile.Columns()
	lastID := opts.startID
	totalScanned := 0
	totalUpdated := 0
	totalExtracted := 0
	totalExpected := 0
	totalSourceMissing := 0
	batchNo := 0
	for {
		if opts.maxBatches > 0 && batchNo >= opts.maxBatches {
			break
		}
		rows, err := dao.ContentProfile.Ctx(ctx).
			WhereGT(columns.Id, lastID).
			Fields(columns.Id, columns.PlainText, columns.Province, columns.City, columns.Age, columns.Height, columns.Weight, columns.CupSize).
			OrderAsc(columns.Id).
			Limit(opts.batchSize).
			All()
		if err != nil {
			return fmt.Errorf("query profiles after id %d: %w", lastID, err)
		}
		if rows.IsEmpty() {
			break
		}
		batchNo++
		updates := make(map[int64]g.Map, len(rows))
		anomalies := make([]string, 0)
		batchExpected := 0
		batchExtracted := 0
		batchSourceMissing := 0
		for _, row := range rows {
			id := row[columns.Id].Int64()
			lastID = id
			totalScanned++
			analysis := profileextractor.Analyze(row[columns.PlainText].String())
			for _, missing := range []bool{analysis.HeightSourceEmpty, analysis.WeightSourceEmpty, analysis.CupSourceEmpty} {
				if missing {
					batchSourceMissing++
				}
			}
			fields := analysis.Fields
			issues := auditProfile(analysis)
			if len(issues) > 0 {
				anomalies = append(anomalies, fmt.Sprintf("profile=%d region=%s/%s issues=%s text=%q", id, row[columns.Province].String(), row[columns.City].String(), strings.Join(issues, ","), textSnippet(row[columns.PlainText].String())))
			}
			for _, pair := range []struct {
				mentioned bool
				found     bool
			}{
				{analysis.HeightMentioned, fields.Height > 0},
				{analysis.WeightMentioned, fields.Weight > 0},
				{analysis.CupMentioned, fields.Cup != ""},
			} {
				if pair.mentioned {
					batchExpected++
					if pair.found {
						batchExtracted++
					}
				}
			}
			data := g.Map{}
			if fields.Height > 0 && row[columns.Height].Int() != fields.Height {
				data[columns.Height] = fields.Height
			} else if currentHeight := row[columns.Height].Int(); currentHeight != 0 && (currentHeight < 140 || currentHeight > 210) {
				data[columns.Height] = 0
			}
			if fields.Weight > 0 && row[columns.Weight].Int() != fields.Weight {
				data[columns.Weight] = fields.Weight
			} else if currentWeight := row[columns.Weight].Int(); currentWeight != 0 && (currentWeight < 50 || currentWeight > 300) {
				data[columns.Weight] = 0
			}
			if fields.Cup != "" && profileextractor.NormalizeCup(row[columns.CupSize].String()) != fields.Cup {
				data[columns.CupSize] = fields.Cup
			} else if currentCup := strings.TrimSpace(row[columns.CupSize].String()); currentCup != "" && profileextractor.NormalizeCup(currentCup) == "" {
				data[columns.CupSize] = ""
			}
			if len(data) == 0 {
				continue
			}
			updates[id] = data
		}
		totalExpected += batchExpected
		totalExtracted += batchExtracted
		totalSourceMissing += batchSourceMissing
		coverage := percentage(totalExtracted, totalExpected)
		fmt.Printf("batch=%d lastId=%d scanned=%d changes=%d anomalies=%d sourceMissing=%d coverage=%.2f%% dryRun=%t\n", batchNo, lastID, len(rows), len(updates), len(anomalies), batchSourceMissing, coverage, opts.dryRun)
		for _, anomaly := range anomalies {
			fmt.Println("ANOMALY", anomaly)
		}
		if opts.strict && len(anomalies) > 0 {
			return fmt.Errorf("batch %d stopped before write: %d anomalous profiles; resume with -start-id %d after fixing rules", batchNo, len(anomalies), rows[0][columns.Id].Int64()-1)
		}
		if !opts.dryRun && len(updates) > 0 {
			if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				for id, data := range updates {
					if _, updateErr := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, id).Data(data).Update(); updateErr != nil {
						return fmt.Errorf("update profile %d: %w", id, updateErr)
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		totalUpdated += len(updates)
	}
	fmt.Printf("done lastId=%d scanned=%d changed=%d sourceMissing=%d coverage=%.2f%% dryRun=%t\n", lastID, totalScanned, totalUpdated, totalSourceMissing, percentage(totalExtracted, totalExpected), opts.dryRun)
	return nil
}

func ensureProfileFieldIndexes(ctx context.Context) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS "idx_content_profile_height_active" ON "hg_content_profile" ("height") WHERE "deleted_at" IS NULL AND "height" > 0`,
		`CREATE INDEX IF NOT EXISTS "idx_content_profile_weight_active" ON "hg_content_profile" ("weight") WHERE "deleted_at" IS NULL AND "weight" > 0`,
		`CREATE INDEX IF NOT EXISTS "idx_content_profile_cup_active" ON "hg_content_profile" ("cup_size") WHERE "deleted_at" IS NULL AND "cup_size" <> ''`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return fmt.Errorf("create profile field index: %w", err)
		}
	}
	indexes, err := g.DB().GetAll(ctx, `
SELECT indexname
FROM pg_indexes
WHERE schemaname = 'public'
  AND indexname IN (
    'idx_content_profile_height_active',
    'idx_content_profile_weight_active',
    'idx_content_profile_cup_active'
  )
ORDER BY indexname`)
	if err != nil {
		return fmt.Errorf("verify profile field indexes: %w", err)
	}
	if len(indexes) != len(statements) {
		return fmt.Errorf("profile field index verification failed: got %d, want %d", len(indexes), len(statements))
	}
	for _, index := range indexes {
		fmt.Println("index ready", index["indexname"].String())
	}
	return nil
}

func auditProfile(analysis profileextractor.Analysis) []string {
	issues := make([]string, 0)
	if analysis.HeightMentioned && !analysis.HeightSourceEmpty && analysis.Height == 0 {
		issues = append(issues, "height_unparsed")
	}
	if analysis.WeightMentioned && !analysis.WeightSourceEmpty && analysis.Weight == 0 {
		issues = append(issues, "weight_unparsed")
	}
	if analysis.CupMentioned && !analysis.CupSourceEmpty && analysis.Cup == "" {
		issues = append(issues, "cup_unparsed")
	}
	return issues
}

func textSnippet(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	chars := []rune(text)
	if len(chars) > 180 {
		chars = chars[:180]
	}
	return string(chars)
}

func percentage(found, expected int) float64 {
	if expected == 0 {
		return 100
	}
	return float64(found) * 100 / float64(expected)
}
