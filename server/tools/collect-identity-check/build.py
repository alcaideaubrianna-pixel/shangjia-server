#!/usr/bin/env python3
"""Build a directly executable local regression binary, without go test.

The overlay includes the actual repository cases as ordinary Go source. The
entry point imports only the collection package and DB drivers, never the
application bootstrap or workers. Run the output binary with a loopback PG.
"""
import json
import pathlib
import re
import subprocess
import tempfile

root = pathlib.Path(__file__).resolve().parents[2]
sysdir = root / 'addons/youban_publish/logic/sys'
output = pathlib.Path('/tmp/youban-collect-identity-check')
with tempfile.TemporaryDirectory(prefix='youban-identity-build-') as tmp:
    tmp = pathlib.Path(tmp)
    replace = {}
    tests = []
    for name in ['collect_identity_postgres_test.go', 'collect_identity_replay_test.go']:
        source = sysdir / name
        text = source.read_text()
        tests += re.findall(r'^func (Test\w+)\(t \*testing.T\)', text, re.M)
        target = tmp / name.replace('_test.go', '_cases.go')
        target.write_text(text)
        replace[str(sysdir / target.name)] = str(target)
    harness = tmp / 'collect_identity_binary_entry.go'
    harness.write_text('package sys\nimport("testing";"regexp")\nfunc RunCollectIdentityChecks(){testing.Main(regexp.MatchString,[]testing.InternalTest{'
                       + ','.join('{"'+name+'",'+name+'}' for name in tests)
                       + '},nil,nil)}\n')
    replace[str(sysdir / harness.name)] = str(harness)
    overlay = tmp / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': replace}))
    main = tmp / 'main.go'
    main.write_text('package main\nimport(sys "hotgo/addons/youban_publish/logic/sys";_ "github.com/gogf/gf/contrib/drivers/pgsql/v2")\nfunc main(){sys.RunCollectIdentityChecks()}\n')
    subprocess.run(['go','build','-overlay',str(overlay),'-o',str(output),str(main)],cwd=root,check=True)
print(output)
