#!/usr/bin/env python3
"""Build a standalone regression binary for the actual tokenized channel queue.

No go test, application bootstrap, production credentials or shared Redis.
The fixture starts its own miniredis and runs production claim/settle Lua.
"""
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile

server = Path(__file__).resolve().parents[2]
package = server / "addons/youban_publish/logic/sys"
with tempfile.TemporaryDirectory(prefix="channel-order-") as directory:
    temp = Path(directory)
    replacements = {}
    for name in ("tg_channel_reliable_claim_test.go", "tg_channel_order_retry_test.go"):
        replacements[str(package / ("zz_regression_" + name.removesuffix("_test.go") + ".go"))] = str(package / name)
    runner = temp / "runner.go"
    runner.write_text('''package sys
import ("regexp"; "testing"; _ "github.com/gogf/gf/contrib/nosql/redis/v2"; _ "github.com/gogf/gf/contrib/drivers/pgsql/v2")
func RunChannelQueueOrderRegression() {
 testing.Main(regexp.MatchString, []testing.InternalTest{
  {"TestTelegramChannelOrderedRetry", TestTelegramChannelOrderedRetry},
  {"TestTelegramChannelOrderOwnerWake", TestTelegramChannelOrderOwnerWake},
  {"TestTelegramChannelReliableClaim", TestTelegramChannelReliableClaim},
  {"TestTelegramChannelSameChatBarrierPostgres", TestTelegramChannelSameChatBarrierPostgres},
 }, nil, nil)
}
''')
    replacements[str(package / "zz_channel_order_regression.go")] = str(runner)
    overlay = temp / "overlay.json"
    overlay.write_text(json.dumps({"Replace": replacements}))
    main = temp / "main.go"
    main.write_text('package main\nimport s "hotgo/addons/youban_publish/logic/sys"\nfunc main(){s.RunChannelQueueOrderRegression()}\n')
    binary = temp / "check"
    subprocess.run(["go", "build", "-overlay", str(overlay), "-o", str(binary), str(main)], cwd=server, check=True)
    pg_bin = Path(os.environ.get("CHANNEL_ORDER_PG_BIN", "/opt/homebrew/opt/postgresql@17/bin"))
    def pg(name):
        candidate = pg_bin / name
        return str(candidate) if candidate.is_file() else shutil.which(name) or name
    data = temp / "pg"
    subprocess.run([pg("initdb"), "-D", str(data), "-U", "channel_order_fixture", "-A", "trust", "--no-locale"],
                   check=True, stdout=subprocess.DEVNULL)
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        port = str(probe.getsockname()[1])
    subprocess.run([pg("pg_ctl"), "-D", str(data), "-l", str(temp / "pg.log"),
                    "-o", f"-h 127.0.0.1 -p {port} -k /tmp", "-w", "start"], check=True)
    try:
        environment = dict(os.environ, CHANNEL_ORDER_FIXTURE_PG_PORT=port)
        result = subprocess.run([str(binary), "-test.v"], cwd=temp, env=environment, text=True, capture_output=True)
        if result.returncode:
            print(result.stdout)
            print(result.stderr)
            result.check_returncode()
        else:
            for line in result.stdout.splitlines():
                if line.startswith(("=== RUN", "--- PASS", "    --- PASS", "PASS")):
                    print(line)
    finally:
        subprocess.run([pg("pg_ctl"), "-D", str(data), "-m", "fast", "-w", "stop"], check=True)
