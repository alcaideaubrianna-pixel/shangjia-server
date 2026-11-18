#!/usr/bin/env python3
"""Read-only, source-scoped raw-message export through the PG host's local psql."""
import argparse
import atexit
import json
import os
import queue
import threading
from pathlib import Path
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--host', required=True)
    parser.add_argument('--identity', required=True)
    parser.add_argument('--database', default='railway')
    parser.add_argument('--tenant-id', type=int, required=True)
    parser.add_argument('--account-id', type=int, required=True)
    parser.add_argument('--source-id', type=int, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--batch-size', type=int, default=200)
    parser.add_argument('--resume', action='store_true', help='continue the same scoped read-only export')
    args = parser.parse_args()
    if min(args.tenant_id, args.account_id, args.source_id) <= 0 or not 1 <= args.batch_size <= 1000:
        parser.error('positive scope IDs and batch-size 1..1000 required')
    if not args.database.replace('_', '').isalnum():
        parser.error('invalid database name')
    if not args.resume and any((args.output / name).exists() for name in ('summary.json', 'raw.jsonl', 'manifest.jsonl')):
        parser.error('output already contains an export; use --resume or a new directory')
    args.output.mkdir(parents=True, exist_ok=True, mode=0o700)
    os.chmod(args.output, 0o700)

    def query(sql):
        for attempt in range(3):
            try:
                return query_once(sql)
            except (subprocess.TimeoutExpired, RuntimeError) as error:
                if attempt == 2:
                    raise RuntimeError('read-only export interrupted; resume from the existing directory') from error
                time.sleep(1)

    connection = None
    output_lines = None
    error_path = args.output / 'export-errors.log'
    error_fd = os.open(error_path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    error_log = os.fdopen(error_fd, 'a')

    def close_connection():
        nonlocal connection
        if connection is not None:
            connection.terminate()
            connection = None

    atexit.register(close_connection)

    def query_once(sql):
        nonlocal connection, output_lines
        if connection is None or connection.poll() is not None:
            connection = subprocess.Popen(
                ['ssh', '-i', args.identity, '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10',
                 '-o', 'ControlMaster=no', '-o', 'ControlPath=none',
                 '-o', 'ServerAliveInterval=10', '-o', 'ServerAliveCountMax=2', args.host,
                 'sudo -u postgres env LC_ALL=C stdbuf -oL psql -X -qAt -v ON_ERROR_STOP=1 -d ' + args.database],
                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=error_log, text=True, bufsize=1)
            output_lines = queue.Queue()
            def read_lines(process, destination):
                for line in process.stdout:
                    destination.put(line)
                destination.put(None)
            threading.Thread(target=read_lines, args=(connection, output_lines), daemon=True).start()
        marker = '__COLLECT_EXPORT_END__'
        connection.stdin.write("SET statement_timeout='3s'; SET default_transaction_read_only=on;\n" + sql + ';\n\\echo ' + marker + '\n')
        connection.stdin.flush()
        rows = []
        try:
            while True:
                line = output_lines.get(timeout=30)
                if line is None:
                    raise RuntimeError('read-only PG session ended; see private export-errors.log')
                if line.strip() == marker:
                    return rows
                if line.startswith('{'):
                    rows.append(json.loads(line))
        except (queue.Empty, RuntimeError):
            close_connection()
            raise RuntimeError('read-only PG session interrupted; see private export-errors.log') from None

    def write_json(path, value):
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, 'w') as stream:
            json.dump(value, stream, ensure_ascii=False, indent=2)

    scope = (args.tenant_id, args.account_id, args.source_id)
    source = query("SELECT json_build_object('id',id) FROM hg_youban_publish_collect_source "
                   "WHERE tenant_id=%d AND account_id=%d AND id=%d AND deleted_at IS NULL "
                   "AND collect_enabled=1 AND status=1" % scope)
    if len(source) != 1:
        raise RuntimeError('enabled source ownership does not match requested scope')
    # Small configuration tables only. Each raw-message page below has an exact
    # source/chat/status and primary-key cursor; never enumerate the raw table.
    channels = query("""SELECT DISTINCT jsonb_build_object('chatId',c.telegram_chat_id) AS item
        FROM hg_youban_publish_collect_account_channel r
        JOIN hg_youban_publish_collect_channel c ON c.id=r.channel_id
        WHERE r.tenant_id=%d AND r.account_id=%d AND r.source_id=%d
          AND r.deleted_at IS NULL AND c.deleted_at IS NULL
          AND r.collect_enabled=1 AND r.status='enabled' AND r.approval_status='approved'
          AND c.telegram_chat_id<>'0' ORDER BY item""" % scope)
    targets = query("SELECT json_build_object('chatId',target_chat_id) FROM hg_youban_publish_channel "
                    "WHERE tenant_id=%d AND publish_direction='up' AND status=1" % args.tenant_id)

    def canonical(chat):
        return chat[4:] if chat.startswith('-100') else chat

    excluded = {canonical(row['chatId']) for row in targets}
    stats = {'tenantId': args.tenant_id, 'accountId': args.account_id, 'sourceId': args.source_id,
             'startedAt': time.strftime('%Y-%m-%dT%H:%M:%S%z'), 'channels': [],
             'excludedPublishChats': [], 'rawMessages': 0,
             'scope': 'received/failed_retry raw messages in enabled approved non-target channels',
             'context': 'raw facts only; pairing must use existing server context, not pending-only adjacency'}
    existing = []
    if args.resume:
        previous = json.loads((args.output / 'summary.json').read_text())
        if any(previous.get(k) != stats[k] for k in ('tenantId', 'accountId', 'sourceId')):
            raise RuntimeError('export resume scope mismatch')
        stats = previous
        with (args.output / 'raw.jsonl').open() as stream:
            existing = [json.loads(line) for line in stream]
        if len({r['id'] for r in existing}) != len(existing):
            raise RuntimeError('duplicate IDs in partial raw export')
        if any(r['tenantId'] != args.tenant_id or r['sourceId'] != args.source_id for r in existing):
            raise RuntimeError('raw export scope mismatch')
        # raw.jsonl is the authoritative export; rebuild the ID-only projection
        # if interruption occurred between its append and the manifest append.
        with (args.output / 'manifest.jsonl').open('w') as stream:
            for row in existing:
                stream.write(json.dumps({k: row[k] for k in ('id', 'tenantId', 'sourceId', 'chatId', 'messageId')}) + '\n')
        stats['rawMessages'] = len(existing)
    write_json(args.output / 'summary.json', stats)
    flags = os.O_WRONLY | os.O_CREAT | (os.O_APPEND if args.resume else os.O_EXCL)
    raw_fd = os.open(args.output / 'raw.jsonl', flags, 0o600)
    manifest_fd = os.open(args.output / 'manifest.jsonl', flags, 0o600)
    with os.fdopen(raw_fd, 'w') as raw_file, os.fdopen(manifest_fd, 'w') as manifest:
        seen_chats = set()
        for channel in channels:
            chat = canonical(channel['chatId'])
            if chat in seen_chats or any(canonical(item['chatId']) == chat for item in stats['channels']):
                continue
            seen_chats.add(chat)
            if chat in excluded:
                stats['excludedPublishChats'].append(channel['chatId'])
                continue
            if not chat.isdigit():
                raise RuntimeError('non-numeric Telegram channel requires explicit review')
            total = sum(1 for row in existing if canonical(row['chatId']) == chat)
            for stored_chat in (chat, '-100' + chat):
                for status in ('received', 'failed_retry'):
                    cursor = max((row['id'] for row in existing if row['chatId'] == stored_chat and row['status'] == status), default=0)
                    # Fixed high water makes a finite export even as webhook traffic arrives.
                    high = query("SELECT json_build_object('id',id) FROM hg_tg_collector_event "
                                 "WHERE source_id=%d AND chat_id='%s' AND status='%s' "
                                 "ORDER BY id DESC LIMIT 1" % (args.source_id, stored_chat, status))
                    upper = high[0]['id'] if high else 0
                    while cursor < upper:
                        rows = query("""SELECT json_build_object('id',id,'tenantId',tenant_id,
                            'sourceId',source_id,'chatId',chat_id,'messageId',message_id,
                            'status',status,'receivedAt',received_at,'rawUpdate',raw_update)
                            FROM hg_tg_collector_event
                            WHERE source_id=%d AND chat_id='%s' AND status='%s'
                              AND tenant_id=%d AND message_id>0 AND id>%d AND id<=%d
                            ORDER BY id LIMIT %d""" % (args.source_id, stored_chat, status,
                                                      args.tenant_id, cursor, upper, args.batch_size))
                        if not rows:
                            break
                        for row in rows:
                            raw_file.write(json.dumps(row, ensure_ascii=False) + '\n')
                            manifest.write(json.dumps({k: row[k] for k in
                                ('id', 'tenantId', 'sourceId', 'chatId', 'messageId')}) + '\n')
                        cursor = rows[-1]['id']
                        total += len(rows)
                        stats['rawMessages'] += len(rows)
                        raw_file.flush()
                        manifest.flush()
                        write_json(args.output / 'summary.json', stats)
                        time.sleep(0.1)
            stats['channels'].append({'chatId': channel['chatId'], 'rawMessages': total})
            print(json.dumps({'chatId': channel['chatId'], 'exported': total}), flush=True)
            write_json(args.output / 'summary.json', stats)
    stats['finishedAt'] = time.strftime('%Y-%m-%dT%H:%M:%S%z')
    write_json(args.output / 'summary.json', stats)
    print(json.dumps({'exported': stats['rawMessages'], 'output': str(args.output)}))


if __name__ == '__main__':
    main()
