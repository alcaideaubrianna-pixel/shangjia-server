#!/usr/bin/env python3
"""Read-only authenticated note-list latency probe; never prints credentials/content."""
import argparse
import json
import math
import os
import time
import urllib.error
import urllib.parse
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-url', default='https://api.xiaohuiji.cc')
    parser.add_argument('--token-file', help='File containing the existing login token')
    parser.add_argument('--header', default='Authorization')
    parser.add_argument('--prefix', default='Bearer ')
    parser.add_argument('--repeat', type=int, default=3)
    parser.add_argument('--cases-file', help='JSON object mapping case names to query parameters')
    args = parser.parse_args()
    token = os.environ.get('YOUBAN_PROBE_TOKEN', '').strip()
    if args.token_file:
        with open(args.token_file, encoding='utf-8') as handle:
            token = handle.read().strip()
    if not token:
        parser.error('Provide an existing login token via --token-file or YOUBAN_PROBE_TOKEN')
    if not 1 <= args.repeat <= 10:
        parser.error('--repeat must be between 1 and 10')
    cases = {
        'collected_created': {},
        'collected_published': {'sortBy': 'publishedAt'},
        'manual_created': {'sourceScope': 'manual'},
        'all_created': {'sourceScope': 'all'},
        'mine_created': {'accountScope': 'mine'},
        'following_created': {'accountScope': 'following'},
        'empty_keyword': {'keyword': '__note_probe_no_match_20261020__'},
    }
    if args.cases_file:
        with open(args.cases_file, encoding='utf-8') as handle:
            cases = json.load(handle)
    # Reject redirects to avoid forwarding credentials to another host.
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, req, fp, code, msg, headers, newurl):
            return None
    opener = urllib.request.build_opener(NoRedirect())
    failed = False
    for name, overrides in cases.items():
        timings = []
        for _ in range(args.repeat):
            params = dict(accountScope='all', sourceScope='collected', sortBy='createdAt', page=1, perPage=20)
            params.update(overrides)
            url = args.base_url.rstrip('/') + '/api/youban_publish/publish/admin/note/list?' + urllib.parse.urlencode(params)
            request = urllib.request.Request(url, headers={args.header: args.prefix + token})
            started = time.monotonic()
            try:
                with opener.open(request, timeout=30) as response:
                    status = response.status
                    payload = json.load(response)
                elapsed = round((time.monotonic() - started) * 1000, 1)
                timings.append(elapsed)
                data = payload.get('data') or {}
                code = payload.get('code')
                ok = status == 200 and isinstance(data, dict) and isinstance(data.get('list'), list)
                failed |= not ok
                print(json.dumps(dict(case=name, http=status, code=code, ms=elapsed,
                                      count=len(data['list']) if ok else None,
                                      traceID=payload.get('traceID'), ok=ok)), flush=True)
                if not ok:
                    break
            except (urllib.error.URLError, TimeoutError, ValueError) as error:
                failed = True
                print(json.dumps(dict(case=name, error=type(error).__name__,
                                      ms=round((time.monotonic() - started) * 1000, 1))), flush=True)
                break
        if timings:
            ordered = sorted(timings)
            print(json.dumps(dict(case=name, samples=len(timings),
                                  p95_ms=ordered[math.ceil(len(ordered) * .95) - 1], max_ms=max(timings))), flush=True)
        if failed:
            break
    raise SystemExit(1 if failed else 0)


if __name__ == '__main__':
    main()
