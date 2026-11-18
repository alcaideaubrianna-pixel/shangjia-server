#!/usr/bin/env python3
"""Offline-validated business collect_event wake; no DB/Redis/TG access."""
import argparse
from collections import Counter
from datetime import datetime, timezone
import fcntl
import hashlib
import json
import os
from pathlib import Path
import sys
import time

from source351_replay import API as ReplayAPI, EXCLUDED, SCOPE, positive, require, save
from source351_prepare import canonical

ACTION = 'wake_collect_events'
DEFAULT = Path('tmp/collector351-backlog-20261118-v3-business/business-pending.jsonl')
APPROVAL = Path('tmp/collector351-backlog-20261118-v2/summary.json')
STATES = {'pending', 'group_collecting', 'waiting_order', 'prechecked',
          'media_pending', 'media_ready', 'failed'}
PROGRESS = ('pending', 'ignored', 'dispatched', 'processed', 'failed', 'missing')


class API(ReplayAPI):
    def __init__(self, base, timeout):
        super().__init__(base, timeout)
        self.base = base.rstrip('/') + '/repair/'

    def call(self, operation, body=None):
        require(operation in {'plan', 'apply', 'status'}, 'unknown API operation')
        return super().call(operation, body)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load_manifest(manifest, summary_path, approval_path, expected):
    # Read once: subsequent network work uses only this frozen in-memory snapshot.
    raw, summary_raw, approval_raw = (p.read_bytes() for p in
                                      (manifest, summary_path, approval_path))
    require(sha(raw) == expected.lower(), 'manifest SHA256 mismatch')
    summary, approval = json.loads(summary_raw), json.loads(approval_raw)
    require(summary.get('complete') is True and summary.get('failures') == []
            and summary.get('processedAtMustBeNull') is True, 'business export incomplete')
    require(all(type(summary.get(k)) is int and summary[k] == SCOPE[k]
                for k in ('tenantId', 'sourceId')), 'business summary scope mismatch')
    require(all(type(approval.get(k)) is int and approval[k] == v for k, v in SCOPE.items())
            and bool(approval.get('finishedAt')), 'approval summary scope/incomplete')
    require(summary.get('approvalSummarySHA256') == sha(approval_raw), 'approval SHA256 mismatch')
    excluded = EXCLUDED | {canonical(c) for c in approval.get('excludedPublishChats', [])}
    chats = {canonical(c['chatId']) for c in approval['channels']}
    require(bool(chats) and not chats & excluded, 'approval contains publish chat')
    unique, counts, statuses = {}, Counter(), Counter()
    lines = raw.splitlines()
    for line in lines:
        require(bool(line.strip()), 'blank manifest row')
        row = json.loads(line)
        require(positive(row.get('id')) and positive(row.get('sourceMessageId')), 'invalid business/message ID')
        require(all(type(row.get(k)) is int and row[k] == SCOPE[k]
                    for k in ('tenantId', 'sourceId')), 'manifest scope mismatch')
        require(type(row.get('accountId')) is int and row['accountId'] in (0, 615), 'storage account mismatch')
        require(canonical(row.get('sourceChatId')) in chats
                and canonical(row['sourceChatId']) not in excluded, 'non-approved/publish chat')
        require(row.get('status') in STATES and 'processedAt' in row
                and row['processedAt'] is None, 'manifest is not unprocessed business backlog')
        require(row['id'] not in unique or unique[row['id']] == row, 'conflicting duplicate business ID')
        unique[row['id']] = row
        counts[str(row['accountId'])] += 1
        statuses[row['status']] += 1
    require(type(summary.get('businessEvents')) is int and summary['businessEvents'] == len(lines)
            and type(summary.get('uniqueBusinessEvents')) is int
            and summary['uniqueBusinessEvents'] == len(unique), 'business row count mismatch')
    require(summary.get('storageAccountCounts') == dict(counts)
            and summary.get('statusCounts') == dict(statuses), 'summary count mismatch')
    require(all(type(v) is int and v >= 0 for v in
                list(summary['storageAccountCounts'].values()) + list(summary['statusCounts'].values())),
            'invalid storage counts')
    keys = ('id', 'tenantId', 'accountId', 'sourceId', 'sourceChatId',
            'sourceMessageId', 'status', 'processedAt', 'sourceUniqueKey', 'createdAt')
    rows = [{k: unique[i][k] for k in keys if k in unique[i]} for i in sorted(unique)]
    binding = {'version': 1, 'action': ACTION, 'scope': SCOPE, 'manifestSHA256': sha(raw),
               'summarySHA256': sha(summary_raw), 'approvalSHA256': sha(approval_raw), 'chats': sorted(chats)}
    return rows, binding


def check_plan(plan, rows, plan_id=None, preview=False):
    ids = [r['id'] for r in rows]
    require(isinstance(plan, dict) and plan.get('action') == ACTION
            and type(plan.get('candidates')) is int and plan['candidates'] == len(ids),
            'plan action/count mismatch')
    returned = plan.get('eventIds')
    require(isinstance(returned, list) and len(returned) == len(ids)
            and all(positive(i) for i in returned) and set(returned) == set(ids), 'plan business ID mismatch')
    require(isinstance(plan.get('planId'), str) and bool(plan['planId'])
            and (plan_id is None or plan['planId'] == plan_id), 'plan ID mismatch')
    if preview:
        items = plan.get('wakePreview')
        require(isinstance(items, list) and len(items) == len(ids)
                and all(isinstance(i, dict) and positive(i.get('eventId')) for i in items), 'missing wakePreview')
        by_id = {i['eventId']: i for i in items}
        require(set(by_id) == set(ids), 'preview business ID mismatch')
        for row in rows:
            item = by_id[row['id']]
            require(all(type(item.get(k)) is int and item[k] == v for k, v in
                        {'tenantId': 78, 'sourceId': 351, 'ownerAccountId': 615,
                         'accountId': row['accountId'], 'sourceMessageId': row['sourceMessageId']}.items())
                    and item.get('sourceChatId') == row['sourceChatId'], 'preview identity/scope mismatch')
            require(isinstance(item.get('identity'), str) and bool(item['identity'].strip()), 'missing server identity')
            require(item.get('decision') in ('eligible', 'terminal', 'blocked')
                    and isinstance(item.get('status'), str) and bool(item['status']), 'invalid preview decision/status')
            for key in ('sourceUniqueKey', 'createdAt'):
                if key in row and key in item:
                    require(item[key] == row[key], 'preview immutable field mismatch')
        require(isinstance(plan.get('confirmToken'), str) and bool(plan['confirmToken']), 'missing confirmToken')
        expires = datetime.fromisoformat(plan['expiresAt'].replace('Z', '+00:00'))
        require(expires.tzinfo is not None and expires > datetime.now(timezone.utc), 'plan expired')


def check_preview(plan, rows):
    check_plan(plan, rows, preview=True)


def progress(status, size):
    p = status.get('wakeProgress')
    require(isinstance(p, dict) and all(type(p.get(k)) is int and p[k] >= 0
                                      for k in ('total',) + PROGRESS)
            and p['total'] == size and sum(p[k] for k in PROGRESS) == size, 'invalid/missing wakeProgress')
    return {k: p[k] for k in ('total',) + PROGRESS}


def record_status(api, entry, state, checkpoint, timeout=None):
    old_timeout = getattr(api, 'timeout', None)
    if timeout is not None and old_timeout is not None:
        api.timeout = min(old_timeout, max(0.001, timeout))
    try:
        status = api.call('status', {'planId': entry['planId']})
        check_plan(status, entry['rows'], entry['planId'])
        entry['status'] = status.get('status')
        if 'wakeProgress' in status:
            entry['wakeProgress'] = progress(status, len(entry['rows']))
        save(checkpoint, state)
        return status
    finally:
        if old_timeout is not None:
            api.timeout = old_timeout


def reconcile(api, state, checkpoint, wait_seconds, poll_interval):
    entry = state['current']
    deadline = time.monotonic() + wait_seconds
    first = True
    while True:
        remaining = deadline - time.monotonic()
        require(first or remaining > 0, 'running deadline reached; resume original plan')
        status = record_status(api, entry, state, checkpoint,
                               max(0.001, remaining) if wait_seconds else None)
        first = False
        phase = status.get('status')
        if phase == 'completed':
            failed = status.get('failed', [])
            require(isinstance(failed, list) and not failed, 'completed plan has failed IDs; manual reconciliation')
            state['history'].append(entry)
            state['cursor'] = entry['rows'][-1]['id']
            state['current'] = None
            save(checkpoint, state)
            return
        require(phase == 'running', 'pending/failed/unknown plan; manual reconciliation, no apply retry')
        remaining = deadline - time.monotonic()
        require(remaining > 0, 'running deadline reached; resume original plan')
        time.sleep(min(poll_interval, remaining))


def drain(api, state, checkpoint, seconds, interval):
    deadline = time.monotonic() + seconds
    while True:
        totals = Counter()
        for entry in state['history']:
            remaining = deadline - time.monotonic()
            require(remaining > 0, 'drain deadline reached; latest progress retained')
            status = record_status(api, entry, state, checkpoint, remaining)
            require(status.get('status') == 'completed', 'history plan no longer completed')
            totals.update(progress(status, len(entry['rows'])))
        state['drain'] = {'wakeProgress': dict(totals), 'reason': 'pending/failed/missing remain',
                          'handoffComplete': all(totals[k] == 0 for k in ('pending', 'failed', 'missing'))}
        if state['drain']['handoffComplete']:
            state['drain']['reason'] = 'collect handoff terminal; TG sending not verified'
        save(checkpoint, state)
        if state['drain']['handoffComplete']:
            return
        remaining = deadline - time.monotonic()
        require(remaining > 0, 'drain deadline reached; latest progress retained')
        time.sleep(min(interval, remaining))


def execute(api, rows, binding, checkpoint, batch_size, max_batches, wait, poll, drain_seconds, drain_interval):
    checkpoint = checkpoint.resolve()
    with os.fdopen(os.open(str(checkpoint) + '.lock', os.O_CREAT | os.O_RDWR, 0o600), 'w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        state = json.loads(checkpoint.read_text()) if checkpoint.exists() else {
            'binding': binding, 'cursor': 0, 'current': None, 'history': []}
        require(state['binding'] == binding, 'checkpoint action/manifest/scope/endpoint mismatch')
        save(checkpoint, state)
        try:
            if state['current'] and state['current'].get('planId'):
                reconcile(api, state, checkpoint, wait, poll)
            for _ in range(max_batches):
                if state['current'] is None:
                    batch = [r for r in rows if r['id'] > state['cursor']][:batch_size]
                    if not batch:
                        break
                    state['current'] = {'rows': batch}
                    save(checkpoint, state)
                entry = state['current']
                require(len(entry['rows']) <= batch_size, 'resume batch exceeds batch-size')
                plan = api.call('plan', {**SCOPE, 'action': ACTION,
                                       'eventIds': [r['id'] for r in entry['rows']], 'limit': len(entry['rows'])})
                check_preview(plan, entry['rows'])
                entry.update(planId=plan['planId'], status='apply-unknown',
                             preview=[{k: i[k] for k in ('eventId', 'identity', 'decision', 'status')}
                                      for i in plan['wakePreview']])
                save(checkpoint, state)  # Durable before apply; token stays only in memory.
                api.call('apply', {'planId': plan['planId'], 'confirmToken': plan['confirmToken']})
                reconcile(api, state, checkpoint, wait, poll)
            exhausted = not any(r['id'] > state['cursor'] for r in rows)
            if drain_seconds:
                drain(api, state, checkpoint, drain_seconds, drain_interval)
            state.pop('stopReason', None)
            save(checkpoint, state)
            print(json.dumps({'cursor': state['cursor'], 'manifestExhausted': exhausted,
                              'businessComplete': bool(drain_seconds and exhausted
                                                       and state.get('drain', {}).get('handoffComplete')),
                              'drain': state.get('drain'), 'meaning': 'collect handoff only; not TG sent'}))
        except Exception as error:
            state['stopReason'] = (str(error) if isinstance(error, RuntimeError) else
                                   'validation/API/checkpoint condition; inspect original plan')
            save(checkpoint, state)
            raise


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('manifest', nargs='?', type=Path, default=DEFAULT)
    parser.add_argument('--summary', type=Path)
    parser.add_argument('--approval', type=Path, default=APPROVAL)
    parser.add_argument('--manifest-sha256', required=True)
    parser.add_argument('--checkpoint', type=Path)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--apply', action='store_true')
    mode.add_argument('--dry-run', action='store_true')
    parser.add_argument('--batch-size', type=int, default=200)
    parser.add_argument('--max-batches', type=int, default=11)
    parser.add_argument('--wait-seconds', type=float, default=330)
    parser.add_argument('--poll-interval', type=float, default=2)
    parser.add_argument('--drain-seconds', type=float, default=0)
    parser.add_argument('--drain-interval', type=float, default=10)
    parser.add_argument('--timeout', type=float, default=45)
    parser.add_argument('--base-url', default=os.environ.get('YOUBAN_AI_OPS_BASE_URL',
                        'https://api.xiaohuiji.cc/api/youban_publish/ai-ops'))
    args = parser.parse_args(argv)
    require(1 <= args.batch_size <= 200 and 1 <= args.max_batches <= 1000
            and 0 <= args.wait_seconds <= 600 and 1 <= args.poll_interval <= 30
            and 0 <= args.drain_seconds <= 86400 and 10 <= args.drain_interval <= 60
            and 0 < args.timeout <= 120, 'invalid bounds')
    summary = args.summary or args.manifest.with_name('business-summary.json')
    rows, binding = load_manifest(args.manifest, summary, args.approval, args.manifest_sha256)
    binding['baseURL'] = args.base_url.rstrip('/')
    if not args.apply:
        print(json.dumps({'mode': 'dry-run', 'records': len(rows), 'binding': binding,
                          'networkRequests': 0, 'businessComplete': False}))
        return
    require(args.checkpoint is not None, '--apply requires --checkpoint')
    require(args.checkpoint.resolve() not in {p.resolve() for p in (args.manifest, summary, args.approval)},
            'checkpoint cannot overwrite inputs')
    execute(API(args.base_url, args.timeout), rows, binding, args.checkpoint,
            args.batch_size, args.max_batches, args.wait_seconds, args.poll_interval,
            args.drain_seconds, args.drain_interval)


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, OSError, ValueError, KeyError, TypeError):
        print('Stopped: validation/API/deadline condition; preserve checkpoint and query original plan. '
              'No automatic apply retry.', file=sys.stderr)
        sys.exit(1)
