#!/usr/bin/env python3
"""Build ordinary Go sources via overlay and run regression binary on disposable PG.
No go test, application bootstrap, inherited database config, or production access.
"""
import json
import os
import pathlib
import re
import socket
import subprocess
import tempfile

root = pathlib.Path(__file__).resolve().parents[2]
pkg = root / "addons/youban_publish/logic/sys"
fixtures = ["tg_outbox_dispatch_retry_postgres_test.go", "tg_timeout_cleanup_finish_postgres_test.go", "tg_terminal_cleanup_postgres_test.go", "tg_bot_membership_failure_postgres_test.go"]
pgbin = pathlib.Path(os.environ.get("TERMINAL_CHECK_PG_BIN", "/opt/homebrew/opt/postgresql@17/bin"))
for command in ("initdb", "pg_ctl"):
    if not (pgbin / command).is_file():
        raise SystemExit(f"missing local PostgreSQL binary: {pgbin / command}")
# Refuse to reuse an existing instance, including any caller's local database.
with socket.socket() as probe:
    try:
        probe.bind(("127.0.0.1", 35432))
    except OSError:
        raise SystemExit("127.0.0.1:35432 already occupied; refusing to reuse a database")
with tempfile.TemporaryDirectory(prefix="youban-terminal-cleanup-") as temp:
    temp = pathlib.Path(temp)
    replace, names = {}, []
    for index, fixture in enumerate(fixtures):
        text = (pkg / fixture).read_text()
        copied = temp / f"fixture{index}.go"
        copied.write_text(text)
        replace[str(pkg / f"zz_terminal_check_fixture{index}.go")] = str(copied)
        if fixture != fixtures[0]:
            names.extend(re.findall(r"^func (Test\w+)\(t \*testing.T\)", text, re.M))
    bridge = temp / "bridge.go"
    tests = ",\n".join(f'{{Name: "{name}", F: {name}}}' for name in names)
    bridge.write_text('package sys\nimport ("regexp"; "testing")\nfunc RunTerminalCleanupCheck() { testing.Main(regexp.MatchString, []testing.InternalTest{'+tests+'}, nil, nil) }\n')
    replace[str(pkg / "zz_terminal_check_bridge.go")] = str(bridge)
    overlay = temp / "overlay.json"
    overlay.write_text(json.dumps({"Replace": replace}))
    main = temp / "main.go"
    main.write_text('package main\nimport (_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"; s "hotgo/addons/youban_publish/logic/sys")\nfunc main(){s.RunTerminalCleanupCheck()}\n')
    binary = temp / "terminal-cleanup-check"
    env = os.environ.copy()
    env["GOMAXPROCS"] = "4"
    # Build before starting PG; syntax conflicts are reported without touching fixtures.
    subprocess.run(["go", "build", "-p", "2", "-overlay", str(overlay), "-o", str(binary), str(main)], cwd=root, env=env, check=True)
    data = temp / "pgdata"
    subprocess.run([str(pgbin / "initdb"), "-D", str(data), "-A", "trust", "-U", "terminal_cleanup", "--no-locale"], stdout=subprocess.DEVNULL, check=True)
    started = False
    try:
        subprocess.run([str(pgbin / "pg_ctl"), "-D", str(data), "-l", str(temp / "postgres.log"), "-o", f"-h 127.0.0.1 -p 35432 -k {temp}", "-w", "start"], check=True)
        started = True
        # Empty environment prevents inherited SAE/PG/service settings entering the binary.
        run_env = {"PATH": os.environ.get("PATH", ""), "USER": "terminal_cleanup", "YOUBAN_LOCAL_TG_OUTBOX": "1", "HOME": str(temp), "TMPDIR": str(temp)}
        subprocess.run([str(binary), "-test.v"], cwd=temp, env=run_env, check=True, timeout=120)
    finally:
        if started:
            subprocess.run([str(pgbin / "pg_ctl"), "-D", str(data), "-m", "immediate", "-w", "stop"], check=True)
