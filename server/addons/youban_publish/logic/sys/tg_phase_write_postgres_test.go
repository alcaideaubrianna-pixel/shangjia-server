package sys

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// This opt-in test creates its own disposable cluster. It never reads an
// application DSN, config group, PGHOST, or production connection environment.
func TestTelegramPhaseWriteLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_PHASE_WRITE_LOCAL_PG") != "1" {
		t.Skip("set YOUBAN_PHASE_WRITE_LOCAL_PG=1 to start a disposable local PostgreSQL")
	}
	const bin = "/opt/homebrew/opt/postgresql@17/bin"
	for _, name := range []string{"initdb", "pg_ctl"} {
		if _, err := os.Stat(filepath.Join(bin, name)); err != nil {
			t.Fatalf("local PostgreSQL binary unavailable: %v", err)
		}
	}
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	run := func(timeout time.Duration, name string, args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(bin, name), args...)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s failed: %w\n%s", name, err, output)
		}
		return nil
	}
	if err := run(30*time.Second, "initdb", "-D", dataDir, "-U", "phase_test", "--auth=trust", "--encoding=UTF8", "--no-locale"); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	// A competing bind causes startup to fail, never a fallback to another DB.
	config := fmt.Sprintf("\nlisten_addresses='127.0.0.1'\nport=%d\nunix_socket_directories=''\nfsync=off\nsynchronous_commit=off\n", port)
	file, err := os.OpenFile(filepath.Join(dataDir, "postgresql.conf"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString(config)
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	// Register before startup so even a failed startup is cleaned up.
	t.Cleanup(func() {
		if _, err := os.Stat(filepath.Join(dataDir, "postmaster.pid")); os.IsNotExist(err) {
			return
		}
		if err := run(20*time.Second, "pg_ctl", "-D", dataDir, "-m", "immediate", "-w", "-t", "10", "stop"); err != nil {
			t.Errorf("stop disposable PostgreSQL: %v", err)
		}
	})
	if err := run(25*time.Second, "pg_ctl", "-D", dataDir, "-l", filepath.Join(root, "postgres.log"), "-w", "-t", "15", "start"); err != nil {
		t.Fatal(err)
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: strconv.Itoa(port), User: "phase_test", Name: "postgres", MaxOpenConnCount: 2, MaxIdleConnCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Close this independent pool before stopping the test-owned cluster.
	t.Cleanup(func() {
		if err := db.Close(context.Background()); err != nil {
			t.Errorf("close phase test DB: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		const table = "phase_write_test"
		if _, err := tx.Exec("CREATE TEMP TABLE " + table + " (id bigint PRIMARY KEY, status text NOT NULL, send_phase text) ON COMMIT DROP"); err != nil {
			return err
		}
		cases := []struct {
			name     string
			source   interface{}
			target   SendPhase
			wantRows int64
		}{
			{"display confirmation", SendPhaseDisplaySending.String(), SendPhaseDisplayConfirmed, 1},
			{"verification confirmation", SendPhaseVerifySending.String(), SendPhaseVerifyConfirmed, 1},
			{"reject confirmed regression", SendPhaseVerifyConfirmed.String(), SendPhaseDisplayConfirmed, 0},
			{"NULL initial phase", nil, SendPhaseCleanupProcessing, 1},
			{"relay definite rejection retry", SendPhasePackageSending.String(), SendPhaseRelayReady, 1},
			{"relay complete verification", SendPhasePackageConfirmed.String(), SendPhaseVerifyConfirmed, 1},
		}
		for i, tc := range cases {
			id := int64(i + 1)
			if _, err := tx.Exec("INSERT INTO "+table+" (id,status,send_phase) VALUES (?, ?, ?)", id, JobStatusSending.String(), tc.source); err != nil {
				return err
			}
			result, err := tx.Model(table).Safe().Ctx(ctx).
				Where("id", id).WhereIn("status", []JobStatus{JobStatusSending}).
				WhereIn("COALESCE(send_phase,'')", telegramPhaseWriteSources(tc.target)).
				Where("send_phase IS DISTINCT FROM ?", tc.target.String()).
				Data(g.Map{"send_phase": tc.target.String()}).Update()
			if err != nil {
				return fmt.Errorf("%s update: %w", tc.name, err)
			}
			rows, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if rows != tc.wantRows {
				return fmt.Errorf("%s: affected=%d want=%d", tc.name, rows, tc.wantRows)
			}
			row, err := tx.GetOne("SELECT send_phase FROM "+table+" WHERE id=?", id)
			if err != nil {
				return err
			}
			want := tc.target.String()
			if tc.wantRows == 0 {
				want = tc.source.(string)
			}
			if row["send_phase"].String() != want {
				return fmt.Errorf("%s: phase=%q want=%q", tc.name, row["send_phase"].String(), want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
