#!/usr/bin/env python3
"""Run on the DB host as postgres. Only hash current original media, never posters.
Dry-run by default. State/audit files contain IDs and digests, never URLs.
"""
import argparse
import concurrent.futures
import hashlib
import json
import os
import signal
import sqlite3
import threading
import time
from urllib.parse import urlparse

import psycopg2
import requests

STOP = threading.Event()


def fingerprint(row, bandwidth, max_bytes):
    media_id, url, path, size, media_type = row
    source = url if urlparse(url or '').scheme in ('http', 'https') else path
    if urlparse(source or '').scheme not in ('http', 'https'):
        return media_id, None, 0, 'no_remote_original'
    digest = hashlib.md5()
    total = 0
    started = time.monotonic()
    try:
        with requests.get(source, stream=True, timeout=(10, 30),
                          headers={'Accept-Encoding': 'identity'}) as response:
            if response.status_code != 200:
                return media_id, None, 0, 'http_' + str(response.status_code)
            content_type = response.headers.get('Content-Type', '').lower()
            if any(x in content_type for x in ('text/', 'json', 'xml')):
                return media_id, None, 0, 'unexpected_content_type'
            for chunk in response.iter_content(256 * 1024):
                if STOP.is_set():
                    return media_id, None, total, 'interrupted'
                total += len(chunk)
                if total > max_bytes or time.monotonic() - started > 600:
                    return media_id, None, total, 'download_limit'
                digest.update(chunk)
                delay = total / bandwidth - (time.monotonic() - started)
                if delay > 0 and STOP.wait(delay):
                    return media_id, None, total, 'interrupted'
        if not total:
            return media_id, None, total, 'empty_original'
        if size and total != size:
            return media_id, None, total, 'size_mismatch'
        return media_id, digest.hexdigest(), total, None
    except Exception as exc:
        # Exception messages may contain signed URLs. Record only their class.
        return media_id, None, total, type(exc).__name__


def apply_one(db, row, digest):
    media_id, url, path, _, _ = row
    with db:
        with db.cursor() as cur:
            cur.execute("SET LOCAL lock_timeout='2s'")
            cur.execute("SET LOCAL statement_timeout='10s'")
            cur.execute("SELECT count(*) FROM hg_youban_publish_media WHERE id=%s AND deleted_at IS NULL AND coalesce(md5,'')='' AND processing_status='ready' AND file_url IS NOT DISTINCT FROM %s AND storage_path IS NOT DISTINCT FROM %s", (media_id,url,path))
            if cur.fetchone()[0] != 1:
                return False
            cur.execute('''UPDATE hg_youban_publish_media SET md5=%s, updated_at=now()
                WHERE id=%s AND deleted_at IS NULL AND coalesce(md5,'')=''
                AND processing_status='ready'
                AND file_url IS NOT DISTINCT FROM %s AND storage_path IS NOT DISTINCT FROM %s
                RETURNING id''', (digest, media_id, url, path))
            if not cur.fetchone():
                return False
            cur.execute('''INSERT INTO hg_youban_publish_media_fingerprint
                (media_id,tenant_id,account_id,profile_id,media_type,phash_bits,md5,deleted_at,updated_at)
                SELECT id,tenant_id,account_id,profile_id,media_type,
                CASE WHEN perceptual_hash ~ '^[0-9a-fA-F]{16}$'
                  THEN ('x'||perceptual_hash)::bit(64) ELSE NULL END,md5,NULL,now()
                FROM hg_youban_publish_media WHERE id=%s AND deleted_at IS NULL
                ON CONFLICT(media_id) DO UPDATE SET md5=EXCLUDED.md5,updated_at=now()
                WHERE hg_youban_publish_media_fingerprint.deleted_at IS NULL
                  AND coalesce(hg_youban_publish_media_fingerprint.md5,'')='' ''', (media_id,))
            cur.execute('''SELECT m.md5,f.md5 FROM hg_youban_publish_media m
                JOIN hg_youban_publish_media_fingerprint f ON f.media_id=m.id
                WHERE m.id=%s AND f.deleted_at IS NULL''', (media_id,))
            actual = cur.fetchone()
            if actual != (digest, digest):
                raise ValueError('fingerprint_conflict')
    return True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--database', default='railway')
    parser.add_argument('--state', default='media-md5-state.sqlite')
    parser.add_argument('--after-id', type=int, default=0)
    parser.add_argument('--through-id', type=int, default=0)
    parser.add_argument('--limit', type=int, default=100, help='maximum candidates per invocation')
    parser.add_argument('--batch-size', type=int, default=50)
    parser.add_argument('--workers', type=int, default=2)
    parser.add_argument('--mbps', type=float, default=4, help='aggregate download MiB/s')
    parser.add_argument('--pause', type=float, default=1)
    parser.add_argument('--retry-failures', action='store_true')
    parser.add_argument('--max-file-mib', type=int, default=1024)
    args = parser.parse_args()
    if not (1 <= args.workers <= 8 and 1 <= args.batch_size <= 500 and args.limit > 0
            and args.mbps > 0 and args.pause >= 0 and args.after_id >= 0):
        parser.error('invalid limits')
    os.umask(0o077)
    signal.signal(signal.SIGTERM, lambda *_: STOP.set())
    signal.signal(signal.SIGINT, lambda *_: STOP.set())
    state = sqlite3.connect(args.state)
    state.execute('CREATE TABLE IF NOT EXISTS audit(id INTEGER PRIMARY KEY,md5 TEXT,bytes INTEGER,status TEXT,at TEXT)')
    state.execute('CREATE TABLE IF NOT EXISTS checkpoint(mode TEXT PRIMARY KEY,last_id INTEGER,high_id INTEGER)')
    mode = 'apply' if args.apply else 'dry_run'
    db = psycopg2.connect(dbname=args.database, application_name='media_md5_backfill')
    db.autocommit = True
    with db.cursor() as cur:
        cur.execute("SET statement_timeout='15s'")
        cur.execute('SELECT pg_try_advisory_lock(160102026, 1308700)')
        if not cur.fetchone()[0]:
            raise RuntimeError('another_backfill_is_running')
        cur.execute('SELECT coalesce(max(id),0) FROM hg_youban_publish_media')
        high = args.through_id or cur.fetchone()[0]
    saved = state.execute('SELECT last_id,high_id FROM checkpoint WHERE mode=?', (mode,)).fetchone()
    cursor_id = max(args.after_id, saved[0] if saved else 0)
    high = min(high, saved[1]) if saved else high
    retry_ids = [r[0] for r in state.execute("SELECT id FROM audit WHERE status NOT IN ('applied','dry_run','race_skipped') ORDER BY id")] if args.retry_failures else []
    attempted = applied = failed = 0
    print(json.dumps(dict(event='start', mode=mode, after_id=cursor_id, high_id=high,
                          limit=args.limit, workers=args.workers, mbps=args.mbps)), flush=True)
    db.autocommit = False
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
        while not STOP.is_set() and attempted < args.limit:
            with db:
                with db.cursor() as cur:
                    fields = 'id,file_url,storage_path,size,media_type'
                    if args.retry_failures:
                        take = min(args.batch_size, args.limit-attempted)
                        ids, retry_ids = retry_ids[:take], retry_ids[take:]
                        if not ids:
                            break
                        cur.execute('SELECT '+fields+''' FROM hg_youban_publish_media
                            WHERE id=ANY(%s) AND deleted_at IS NULL AND coalesce(md5,'')=''
                            AND processing_status='ready' ORDER BY id''', (ids,))
                    else:
                        # Scan bounded primary-key windows, even when no candidates exist.
                        end = min(cursor_id + 10000, high)
                        cur.execute('SELECT '+fields+''' FROM hg_youban_publish_media
                            WHERE id>%s AND id<=%s AND deleted_at IS NULL AND coalesce(md5,'')=''
                            AND processing_status='ready' AND media_type IN ('image','video')
                            ORDER BY id LIMIT %s''', (cursor_id, end, min(args.batch_size,args.limit-attempted)))
                    rows = cur.fetchall()
            if not rows:
                if args.retry_failures:
                    continue
                cursor_id = end
                state.execute('INSERT OR REPLACE INTO checkpoint VALUES (?,?,?)', (mode,cursor_id,high))
                state.commit()
                if cursor_id >= high:
                    break
                continue
            tasks = [pool.submit(fingerprint, row, args.mbps*1024*1024/args.workers,
                                 args.max_file_mib*1024*1024) for row in rows]
            for row, task in zip(rows, tasks):
                media_id, digest, size, error = task.result()
                if error == 'interrupted':
                    break
                status = error or 'dry_run'
                if not error and args.apply:
                    try:
                        status = 'applied' if apply_one(db,row,digest) else 'race_skipped'
                    except Exception as exc:
                        status = 'db_' + type(exc).__name__
                attempted += 1
                applied += status == 'applied'
                failed += status not in ('applied','dry_run','race_skipped')
                state.execute('INSERT OR REPLACE INTO audit VALUES (?,?,?,?,datetime(\'now\'))',
                              (media_id,digest,size,status))
                if not args.retry_failures:
                    cursor_id = media_id
                    state.execute('INSERT OR REPLACE INTO checkpoint VALUES (?,?,?)', (mode,cursor_id,high))
                state.commit()
            print(json.dumps(dict(event='batch',last_id=cursor_id,attempted=attempted,
                                  applied=applied,failed=failed)), flush=True)
            STOP.wait(args.pause)
    db.close()
    state.close()
    print(json.dumps(dict(event='stopped' if STOP.is_set() else 'complete',last_id=cursor_id,
                          attempted=attempted,applied=applied,failed=failed)), flush=True)


if __name__ == '__main__':
    main()
