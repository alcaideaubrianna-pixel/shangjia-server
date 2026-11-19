#!/usr/bin/env python3
"""Disposable loopback PostgreSQL admission fixture. Builds/runs a binary; no go test.
The real query/merge/admission functions are used. Success persistence is a
minimal fixture sink, not full production commit/dispatch end-to-end coverage.
"""
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile

here = Path(__file__).resolve().parent
server = here.parents[1]
package = server / 'addons/youban_publish/logic/sys'

pg_bin = Path(os.environ.get("COLLECT_ADMISSION_PG_BIN", "/opt/homebrew/opt/postgresql@17/bin"))
if not (pg_bin / "postgres").is_file():
    raise SystemExit("Set COLLECT_ADMISSION_PG_BIN to a PostgreSQL server bin directory")

def run(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)

with tempfile.TemporaryDirectory(prefix='collect-admission-pg-') as directory:
    temp = Path(directory)
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = str(sock.getsockname()[1])
    data = temp / 'data'
    # No inherited PG connection settings or application config are loaded.
    env = {k: v for k, v in os.environ.items() if not k.startswith(('PG', 'GF_', 'YOUBAN_'))}
    env['COLLECT_ADMISSION_LOCAL_PORT'] = port
    run([str(pg_bin / 'initdb'), '-D', str(data), '-U', 'admission_fixture', '-A', 'trust', '--no-locale'], env=env, stdout=subprocess.DEVNULL)
    started = False
    try:
        run([str(pg_bin / 'pg_ctl'), '-D', str(data), '-l', str(temp / 'postgres.log'), '-o', f'-h 127.0.0.1 -p {port} -k {temp}', '-w', 'start'], env=env, stdout=subprocess.DEVNULL)
        started = True
        # Replace only the external enqueue side effect in this test binary.
        # Pairing/readiness, event updates and prepare/commit gates remain real.
        queue_path = package / 'collect_media_queue.go'
        queue_source = queue_path.read_text()
        enqueue_body = '''func (s *sSysPublish) enqueueCollectMediaCache(ctx context.Context, payload collectMediaQueuePayload, delay time.Duration) error {
	_, err := s.enqueueCollectMediaCacheTask(ctx, payload, delay)
	return err
}'''
        if queue_source.count(enqueue_body) != 1:
            raise SystemExit('enqueue boundary changed; review fixture overlay')
        queue_stub = temp / 'collect_media_queue.go'
        queue_stub.write_text(queue_source.replace(enqueue_body, '''func (s *sSysPublish) enqueueCollectMediaCache(ctx context.Context, payload collectMediaQueuePayload, delay time.Duration) error {
 admissionFixtureEnqueued = append(admissionFixtureEnqueued, payload.EventId)
 return nil
}'''))
        process_path = package / 'collect_process_queue.go'
        process_source = process_path.read_text()
        deferred = """func (s *sSysPublish) enqueueCollectProcessDeferred(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration) (bool, error) {
	return s.enqueueCollectProcessTask(ctx, payload, delay, false)
}"""
        if process_source.count(deferred) != 1:
            raise SystemExit('deferred enqueue boundary changed; review fixture')
        process_stub = temp / 'collect_process_queue.go'
        process_stub.write_text(process_source.replace(deferred, """func (s *sSysPublish) enqueueCollectProcessDeferred(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration) (bool, error) {
 err := ensureCollectProcessOutbox(ctx, payload, delay)
 return err == nil, err
}"""))
        overlay = temp / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': {str(package / 'zz_collect_admission_pg_fixture.go'): str(here / 'postgres_fixture.go.txt'), str(queue_path): str(queue_stub), str(process_path): str(process_stub)}}))
        main = temp / 'main.go'
        main.write_text('package main\nimport s "hotgo/addons/youban_publish/logic/sys"\nfunc main(){s.RunCollectAdmissionPostgresFixture()}\n')
        binary = temp / 'check'
        run(['go', 'build', '-overlay', str(overlay), '-o', str(binary), str(main)], cwd=server, env=env)
        run([str(binary)], cwd=temp, env=env)
    finally:
        if started:
            run([str(pg_bin / 'pg_ctl'), '-D', str(data), '-m', 'immediate', '-w', 'stop'], env=env, stdout=subprocess.DEVNULL)
