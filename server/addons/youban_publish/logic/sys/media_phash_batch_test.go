package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestMediaPHashBatchSQLBounds(t *testing.T) {
	scopes := []mediaPHashBucketScopePart{{TenantId: 5, AccountIds: []int64{6}}}
	for _, test := range []struct {
		hashes    []string
		threshold int
	}{
		{[]string{"invalid"}, 12}, {[]string{"0000000000000000"}, -1},
		{[]string{"0000000000000000"}, 16}, {make([]string, mediaPHashBatchSize+1), 12},
	} {
		if _, _, err := mediaPHashBatchSQL(test.hashes, test.threshold, scopes, false); err == nil {
			t.Fatalf("accepted invalid input: %+v", test)
		}
	}
	query, _, err := mediaPHashBatchSQL([]string{"0000000000000000"}, 12, nil, false)
	if err != nil || query != "" {
		t.Fatalf("unscoped query=%q err=%v", query, err)
	}
}

// Real PostgreSQL execution, not SQL string matching. Both strategies must
// reproduce each single-input query, including aliases, bucket counts, deleted
// media and tenant/account isolation. All writes are temporary and local.
func TestMediaPHashBatchLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres", MaxOpenConnCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	noiseRows := 20000
	if value := os.Getenv("YOUBAN_SCAN_BATCH_NOISE_ROWS"); value != "" {
		noiseRows, err = strconv.Atoi(value)
		if err != nil || noiseRows < 1 || noiseRows > 200000 {
			t.Fatal("noise rows must be between 1 and 200000")
		}
	}
	incidentCases := localBatchIncidentHashes(t)
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		ddl := []string{
			`CREATE TEMP TABLE hg_content_profile(id bigint PRIMARY KEY,status int,visibility int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_media(id bigint PRIMARY KEY,profile_id bigint,tenant_id bigint,account_id bigint,media_type text,perceptual_hash text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_note_index(profile_id bigint,tenant_id bigint,account_id bigint,status int,visibility int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE INDEX ON hg_youban_publish_profile_state(profile_id,tenant_id,account_id)`,
			`CREATE INDEX ON hg_youban_publish_note_index(profile_id,tenant_id,account_id)`,
		}
		for _, table := range []string{publishMediaPHashLshTable, publishMediaPHashAliasBucketTable} {
			ddl = append(ddl, fmt.Sprintf(`CREATE TEMP TABLE %s(media_id bigint,profile_id bigint,tenant_id bigint,account_id bigint,media_type text,hash_value text,bucket_pos int,bucket_value int) ON COMMIT DROP`, table),
				fmt.Sprintf(`CREATE INDEX ON %s(tenant_id,account_id,bucket_pos,bucket_value)`, table))
		}
		for _, sql := range ddl {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		// Noise is deterministic, so both implementations see identical data.
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media SELECT n,n,5,6,'image',substring(md5(n::text),1,16),NULL FROM generate_series(1,?) n`, noiseRows); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media VALUES
 (300001,300001,5,6,'image','000f000f000f0000',NULL),
 (300002,300002,5,6,'image','000f000f000f0001',NULL),
 (300003,300003,9,6,'image','000f000f000f0000',NULL),
 (300004,300004,5,7,'image','000f000f000f0000',NULL),
 (300005,300005,5,6,'image','000f000f000f0000',now()),
 (300006,300006,5,6,'image','000f000f000f0000',NULL),
 (300007,300007,5,6,'image','000f000f000f0000',NULL)`); err != nil {
			return err
		}
		// Give original query hashes real positive matches in this synthetic
		// corpus; timing an empty result alone misses permission/hydration work.
		seed := int64(1000000)
		for _, hashes := range incidentCases {
			for _, hash := range hashes {
				if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media
 SELECT ?+n,?+n,5,6,'image',lpad(to_hex((('x'||?)::bit(64) # n::bit(64))::bigint),16,'0'),NULL
 FROM generate_series(1,128) n`, seed, seed, hash); err != nil {
					return err
				}
				seed += 256
			}
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_profile_state SELECT id,tenant_id,account_id,NULL FROM hg_youban_publish_media WHERE id<>300006`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_content_profile SELECT id,1,1,NULL FROM hg_youban_publish_media`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_note_index SELECT id,tenant_id,account_id,1,1,NULL FROM hg_youban_publish_media WHERE id<>300007`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media_phash_lsh
 SELECT id,id,tenant_id,account_id,'image',perceptual_hash,p,(substring(('x'||perceptual_hash)::bit(64) from (p-1)*16+1 for 16))::int
 FROM hg_youban_publish_media CROSS JOIN generate_series(1,4) p`); err != nil {
			return err
		}
		// One alias duplicates the original; another is a distinct fingerprint.
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media_phash_alias_bucket SELECT * FROM hg_youban_publish_media_phash_lsh WHERE media_id=300001`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media_phash_alias_bucket
 SELECT 1,1,5,6,'image','0000000000000001',p, (substring(('x'||'0000000000000001')::bit(64) from (p-1)*16+1 for 16))::int FROM generate_series(1,4) p`); err != nil {
			return err
		}
		for _, table := range []string{publishMediaPHashLshTable, publishMediaPHashAliasBucketTable} {
			if _, err := tx.Exec("ANALYZE " + table); err != nil {
				return err
			}
		}
		scopes := []mediaPHashBucketScopePart{{TenantId: 5, AccountIds: []int64{6}}, {TenantId: 8, AccountIds: []int64{99}}}
		for account := int64(1000); account <= 1500; account++ {
			scopes[0].AccountIds = append(scopes[0].AccountIds, account)
		}
		hashes := []string{"0000000000000000", "0000000000000001", "ffffffffffffffff", "000f000f000f0000", "1234567890abcdef", "a2995926ca37967a", "a2995926ca37967b", "0000000000000000"}
		identity := func(rows []mediaPHashBucketCandidateRow) []string {
			keys := make([]string, 0, len(rows))
			for _, row := range rows {
				keys = append(keys, fmt.Sprintf("%d:%d:%d:%d:%s:%s:%d", row.MediaId, row.ProfileId, row.TenantId, row.AccountId, row.MediaType, row.HashValue, row.BucketHits))
			}
			sort.Strings(keys)
			return keys
		}
		cases := incidentCases
		cases["synthetic"] = hashes
		for name, hashes := range cases {
			for _, scanScope := range []bool{false, true} {
				thresholds := []int{12}
				if name == "synthetic" {
					thresholds = []int{0, 12, 14, 15}
				}
				for _, threshold := range thresholds {
					started := time.Now()
					query, args, err := mediaPHashBatchSQL(hashes, threshold, scopes, scanScope)
					if err != nil {
						return err
					}
					var batch []mediaPHashBucketCandidateRow
					if err := tx.Raw(query, args...).Scan(&batch); err != nil {
						return err
					}
					elapsed := time.Since(started)
					groups := make([][]mediaPHashBucketCandidateRow, len(hashes))
					for _, row := range batch {
						if row.InputIndex < 0 || row.InputIndex >= len(hashes) {
							t.Fatalf("invalid input index %d", row.InputIndex)
						}
						groups[row.InputIndex] = append(groups[row.InputIndex], row)
					}
					if name != "synthetic" {
						for i, group := range groups {
							if len(group) == 0 {
								t.Fatalf("case=%s input=%d lost seeded positive matches", name, i)
							}
						}
					}
					started = time.Now()
					for i, hash := range hashes {
						query, args, err := mediaPHashLshCandidateSQLWithStrategy(hash, threshold, scopes, nil, "image", 0, scanScope)
						if err != nil {
							return err
						}
						var single []mediaPHashBucketCandidateRow
						if err := tx.Raw(query, args...).Scan(&single); err != nil {
							return err
						}
						if !reflect.DeepEqual(identity(groups[i]), identity(single)) {
							t.Fatalf("strategy=%v threshold=%d input=%d batch=%v single=%v", scanScope, threshold, i, identity(groups[i]), identity(single))
						}
					}
					t.Logf("case=%s noise=%d scopeScan=%v threshold=%d inputs=%d batch=%s sequential=%s candidates=%d", name, noiseRows, scanScope, threshold, len(hashes), elapsed, time.Since(started), len(batch))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Only original input hashes are reused here. The candidate corpus remains
// synthetic: these timings must never be reported as full production replay.
func localBatchIncidentHashes(t *testing.T) map[string][]string {
	t.Helper()
	result := make(map[string][]string)
	path := os.Getenv("YOUBAN_SCAN_INCIDENT_FIXTURE")
	if path == "" {
		return result
	}
	var fixture struct {
		Inputs []struct {
			TraceID string `json:"traceId"`
			Item    struct {
				FileUniqueID string `json:"fileUniqueId"`
			} `json:"item"`
		} `json:"inputs"`
		Fingerprints map[string]string `json:"fingerprints"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, input := range fixture.Inputs {
		switch input.TraceID {
		case "ICFBUMHJ", "FPKMXDZB", "YOEKMFUD", "7LYKUCBD":
		default:
			continue
		}
		var fp cachedBotMediaFingerprint
		if err = json.Unmarshal([]byte(fixture.Fingerprints[botMediaFingerprintCacheKey(input.Item.FileUniqueID)]), &fp); err != nil {
			t.Fatal(err)
		}
		result[input.TraceID] = append(result[input.TraceID], fmt.Sprintf("%016x", fp.PHash))
	}
	for name, count := range map[string]int{"ICFBUMHJ": 8, "FPKMXDZB": 7, "YOEKMFUD": 6, "7LYKUCBD": 5} {
		if len(result[name]) != count {
			t.Fatalf("case=%s original hash count=%d want=%d", name, len(result[name]), count)
		}
	}
	return result
}

func TestMediaPHashBatchSQLUsesSharedScan(t *testing.T) {
	query, _, err := mediaPHashBatchSQL([]string{"0000000000000000", "0000000000000001"}, 12, []mediaPHashBucketScopePart{{TenantId: 5, AccountIds: []int64{6}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(query, "FROM "+publishMediaPHashLshTable+" b") != 1 || !strings.Contains(query, "PARTITION BY c.input_index") {
		t.Fatal("batch must share scan and bound each input independently")
	}
}
