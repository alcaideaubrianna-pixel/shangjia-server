"""Local fixtures and mock API only; never connect to production."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import source351_business_wake as wake


def row(i=1):
    return {'id': i, 'tenantId': 78, 'accountId': 615, 'sourceId': 351,
            'sourceChatId': '-1004411799859', 'sourceMessageId': i + 100,
            'status': 'pending', 'processedAt': None}


def plan(rows, phase='completed'):
    return {'planId': 'p1', 'action': wake.ACTION, 'candidates': len(rows),
            'eventIds': [r['id'] for r in rows], 'status': phase,
            'confirmToken': 'SECRET', 'expiresAt': '2099-01-01T00:00:00Z',
            'wakePreview': [dict(r, eventId=r['id'], identity='server-invariant',
                                 ownerAccountId=615, decision='eligible') for r in rows]}


def pg(**changes):
    return dict(total=1, pending=0, ignored=0, dispatched=0, processed=1,
                failed=0, missing=0, **changes) if not changes else {**pg(), **changes}


class MockAPI:
    timeout = 45

    def __init__(self, rows, statuses=None, fail_apply=False):
        self.rows, self.statuses, self.fail_apply = rows, statuses or [plan(rows)], fail_apply
        self.calls = []

    def call(self, op, body):
        self.calls.append((op, body))
        if op == 'plan':
            return plan(self.rows)
        if op == 'apply':
            if self.fail_apply:
                raise RuntimeError('unknown apply')
            return {'status': 'running'}
        return self.statuses.pop(0) if len(self.statuses) > 1 else self.statuses[0]


class WakeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.cp = self.root / 'checkpoint.json'
        self.rows = [row()]
        self.binding = {'action': wake.ACTION}

    def execute(self, api, drain=0, wait=0):
        with patch('builtins.print'):
            wake.execute(api, self.rows, self.binding, self.cp, 200, 11, wait, 2, drain, 10)

    def fixture(self, rows=None):
        rows = rows or self.rows
        manifest, summary, approval = [self.root / n for n in ('events.jsonl', 'business-summary.json', 'approval.json')]
        approval.write_text(json.dumps({**wake.SCOPE, 'finishedAt': 'done',
                                       'channels': [{'chatId': row()['sourceChatId']}]}))
        manifest.write_text(''.join(json.dumps(r) + '\n' for r in rows))
        summary.write_text(json.dumps({'tenantId': 78, 'sourceId': 351, 'complete': True,
                                      'failures': [], 'processedAtMustBeNull': True,
                                      'approvalSummarySHA256': wake.sha(approval.read_bytes()),
                                      'businessEvents': len(rows), 'uniqueBusinessEvents': len({r['id'] for r in rows}),
                                      'storageAccountCounts': dict(wake.Counter(str(r['accountId']) for r in rows)),
                                      'statusCounts': dict(wake.Counter(r['status'] for r in rows))}))
        return manifest, summary, approval, wake.sha(manifest.read_bytes())

    def test_sort_dedupe_and_storage_zero(self):
        zero = dict(row(2), accountId=0)
        rows, _ = wake.load_manifest(*self.fixture([zero, row(), row()]))
        self.assertEqual([r['id'] for r in rows], [1, 2])

    def test_manifest_prevalidation(self):
        for bad in (dict(row(), tenantId=79), dict(row(), accountId=616),
                    dict(row(), sourceChatId='-1003438550143'), dict(row(), processedAt='done'),
                    dict(row(), id=True), dict(row(), sourceChatId='-1009999999')):
            with self.subTest(bad=bad), self.assertRaises(RuntimeError):
                wake.load_manifest(*self.fixture([row(), bad]))
        with self.assertRaises(RuntimeError):
            wake.load_manifest(*self.fixture([row(), dict(row(), sourceMessageId=999)]))
        args = self.fixture()
        with self.assertRaises(RuntimeError):
            wake.load_manifest(*args[:3], '0' * 64)
        args[2].write_text('{}')
        with self.assertRaises(RuntimeError):
            wake.load_manifest(*args)

    def test_summary_incomplete_counts(self):
        for change in ({'complete': False}, {'businessEvents': 3}, {'storageAccountCounts': {'615': True}}):
            args = self.fixture()
            data = json.loads(args[1].read_text())
            args[1].write_text(json.dumps({**data, **change}))
            with self.assertRaises(RuntimeError):
                wake.load_manifest(*args)

    def test_preview_terminal_blocked_and_identity(self):
        for decision in ('eligible', 'terminal', 'blocked'):
            p = plan(self.rows)
            p['wakePreview'][0]['decision'] = decision
            wake.check_preview(p, self.rows)
            p['wakePreview'][0]['tenantId'] = 79
            with self.assertRaises(RuntimeError):
                wake.check_preview(p, self.rows)
        for change in ({'identity': ''}, {'ownerAccountId': 0}, {'sourceMessageId': 999},
                       {'accountId': 0}, {'eventId': 999}, {'sourceId': 352}):
            p = plan(self.rows)
            p['wakePreview'][0].update(change)
            with self.assertRaises(RuntimeError):
                wake.check_preview(p, self.rows)

    def test_wrong_plan_ids_action_expiry(self):
        for change in ({'eventIds': [999]}, {'eventIds': [True]}, {'action': 'replay_bot_source_updates'},
                       {'candidates': True}, {'expiresAt': '2000-01-01T00:00:00Z'}, {'wakePreview': []}):
            with self.subTest(change=change), self.assertRaises(RuntimeError):
                wake.check_preview({**plan(self.rows), **change}, self.rows)

    def test_unknown_apply_resume_only_status(self):
        api = MockAPI(self.rows, fail_apply=True)
        with self.assertRaises(RuntimeError):
            self.execute(api)
        state = json.loads(self.cp.read_text())
        self.assertEqual(state['current']['planId'], 'p1')
        self.assertNotIn('SECRET', self.cp.read_text())
        api = MockAPI(self.rows)
        self.execute(api)
        self.assertEqual([c[0] for c in api.calls], ['status'])
        self.assertIsNone(json.loads(self.cp.read_text())['current'])

    def test_running_deadline_and_pending_unknown_failed(self):
        for phase in ('running', 'pending', 'failed', 'unknown'):
            with self.subTest(phase=phase):
                cp = self.root / (phase + '.json')
                api = MockAPI(self.rows, [plan(self.rows, phase)])
                with self.assertRaises(RuntimeError):
                    wake.execute(api, self.rows, self.binding, cp, 200, 11, 0, 2, 0, 10)
                self.assertEqual(json.loads(cp.read_text())['cursor'], 0)
                self.assertEqual([c[0] for c in api.calls].count('apply'), 1)

    def test_running_polls_same_plan(self):
        api = MockAPI(self.rows, [plan(self.rows, 'running'), plan(self.rows)])
        with patch.object(wake.time, 'sleep'):
            self.execute(api, wait=330)
        self.assertEqual([b['planId'] for op, b in api.calls if op == 'status'], ['p1', 'p1'])

    def test_status_wrong_id_preserves_current(self):
        api = MockAPI(self.rows, [{**plan(self.rows), 'eventIds': [999]}])
        with self.assertRaises(RuntimeError):
            self.execute(api)
        self.assertEqual(json.loads(self.cp.read_text())['current']['planId'], 'p1')

    def test_progress_integer_partition(self):
        for p in (pg(total=2), pg(pending=True), pg(missing=-1), {}, pg(processed=0)):
            with self.assertRaises(RuntimeError):
                wake.progress({'wakeProgress': p}, 1)

    def test_drain_all_history_only_explicit(self):
        api = MockAPI(self.rows)
        self.execute(api)
        self.assertEqual([c[0] for c in api.calls], ['plan', 'apply', 'status'])
        api = MockAPI(self.rows, [{**plan(self.rows), 'wakeProgress': pg()}])
        self.execute(api, drain=10)
        self.assertEqual([c[0] for c in api.calls], ['status'])
        self.assertTrue(json.loads(self.cp.read_text())['drain']['handoffComplete'])

    def test_drain_missing_or_failed_retains_counts(self):
        self.execute(MockAPI(self.rows))
        for key in ('pending', 'failed', 'missing'):
            status = {**plan(self.rows), 'wakeProgress': pg(processed=0, **{key: 1})}
            api = MockAPI(self.rows, [status])
            with patch.object(wake.time, 'monotonic', side_effect=[0, 0, 0, 20]), patch.object(wake.time, 'sleep'):
                with self.assertRaises(RuntimeError):
                    self.execute(api, drain=10)
            saved = json.loads(self.cp.read_text())
            self.assertEqual(saved['history'][0]['wakeProgress'][key], 1)
            self.assertFalse(saved['drain']['handoffComplete'])

    def test_binding_refuses_raw_checkpoint(self):
        wake.save(self.cp, {'binding': {'action': 'replay_bot_source_updates'}})
        api = MockAPI(self.rows)
        with self.assertRaises(RuntimeError):
            self.execute(api)
        self.assertEqual(api.calls, [])

    def test_offline_default(self):
        manifest, summary, approval, digest = self.fixture()
        with patch.object(wake, 'API', side_effect=AssertionError('network forbidden')), patch('builtins.print'):
            wake.main([str(manifest), '--summary', str(summary), '--approval', str(approval),
                       '--manifest-sha256', digest])
        self.assertFalse(self.cp.exists())

    def test_drain_multiple_history_plans(self):
        second = row(2)
        state = {'history': [{'planId': 'p1', 'rows': self.rows},
                             {'planId': 'p2', 'rows': [second]}]}
        api = MockAPI(self.rows, [{**plan(self.rows), 'wakeProgress': pg()},
                                 {**plan([second]), 'planId': 'p2', 'wakeProgress': pg()}])
        wake.drain(api, state, self.cp, 10, 10)
        self.assertEqual([b['planId'] for op, b in api.calls], ['p1', 'p2'])
        self.assertEqual(state['drain']['wakeProgress']['total'], 2)

    def test_lock_excludes_concurrent_writer(self):
        import fcntl
        with open(str(self.cp) + '.lock', 'w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            api = MockAPI(self.rows)
            with self.assertRaises(BlockingIOError):
                self.execute(api)
            self.assertEqual(api.calls, [])

    def test_2001_business_ids_eleven_bounded_batches(self):
        rows = [row(i) for i in range(1, 2002)]

        class BatchAPI(MockAPI):
            def call(self, op, body):
                self.calls.append((op, body))
                if op == 'plan':
                    self.rows = [r for r in rows if r['id'] in body['eventIds']]
                    self.result = plan(self.rows)
                    self.result['planId'] = 'p' + str(self.rows[0]['id'])
                    return self.result
                return self.result

        api = BatchAPI(rows)
        with patch('builtins.print'):
            wake.execute(api, rows, self.binding, self.cp, 200, 11, 330, 2, 0, 10)
        requests = [b for op, b in api.calls if op == 'plan']
        self.assertEqual([len(b['eventIds']) for b in requests], [200] * 10 + [1])
        self.assertTrue(all(b['action'] == wake.ACTION and b['tenantId'] == 78
                            and b['accountId'] == 615 and b['sourceId'] == 351 for b in requests))
        state = json.loads(self.cp.read_text())
        self.assertEqual(state['cursor'], 2001)
        self.assertEqual(len(state['history']), 11)

    def test_api_route_and_unknown_operation(self):
        with patch.dict('os.environ', {'YOUBAN_AI_OPS_TOKEN': 'test-only'}):
            api = wake.API('https://example.invalid/api/youban_publish/ai-ops', 1)
            self.assertTrue(api.base.endswith('/ai-ops/repair/'))
            with self.assertRaises(RuntimeError):
                api.call('unknown', {})
            with self.assertRaises(RuntimeError):
                wake.API('http://example.invalid', 1)


if __name__ == '__main__':
    unittest.main()
