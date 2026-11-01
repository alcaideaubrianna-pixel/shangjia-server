#!/usr/bin/env python3
"""Read-only media host audit for one publish account.

Reads PostgreSQL credentials from environment, checks each media URL and host
fallback without changing business data. Writes JSON report.
"""
import argparse, json, os, sys, time
from urllib.parse import urlparse, urlunparse

import psycopg2
import requests


def env(name, default=''):
    return os.getenv(name, default)


def build_url_candidates(url, storage_path, hosts):
    values=[]
    raw=(url or '').strip()
    path=(storage_path or '').strip().lstrip('/')
    if raw: values.append(('original', raw))
    if path:
        for label, host in hosts:
            values.append((label, host.rstrip('/') + '/' + path))
    if raw:
        parsed=urlparse(raw)
        if parsed.scheme and parsed.netloc and path:
            for label, host in hosts:
                values.append((label, host.rstrip('/') + '/' + path))
    out=[]; seen=set()
    for label, candidate in values:
        if candidate not in seen:
            seen.add(candidate); out.append((label,candidate))
    return out


def probe(session, url, timeout):
    try:
        r=session.head(url, timeout=timeout, allow_redirects=True)
        if r.status_code in (405, 403, 451):
            r=session.get(url, timeout=timeout, stream=True, allow_redirects=True)
        return {'status': r.status_code, 'final_url': r.url, 'content_length': r.headers.get('content-length',''), 'error': ''}
    except Exception as e:
        return {'status': 0, 'final_url': '', 'content_length': '', 'error': str(e)[:300]}


def main():
    ap=argparse.ArgumentParser()
    ap.add_argument('--account', default='chenguanxi')
    ap.add_argument('--output', default='/tmp/chenguanxi-media-host-audit.json')
    ap.add_argument('--timeout', type=float, default=12)
    ap.add_argument('--limit', type=int, default=0)
    args=ap.parse_args()
    conn=psycopg2.connect(host=env('YOUBAN_DB_HOST','127.0.0.1'), port=int(env('YOUBAN_DB_PORT','5432')), dbname=env('YOUBAN_DB_NAME','railway'), user=env('YOUBAN_DB_USER'), password=env('YOUBAN_DB_PASSWORD'), connect_timeout=15)
    cur=conn.cursor()
    cur.execute("SELECT id,tenant_id,nickname,username FROM hg_youban_publish_account WHERE lower(nickname)=lower(%s) OR lower(username)=lower(%s) ORDER BY id", (args.account,args.account))
    accounts=cur.fetchall()
    if not accounts:
        raise SystemExit(f'account not found: {args.account}')
    ids=[row[0] for row in accounts]
    sql="""SELECT m.id,m.account_id,m.profile_id,m.purpose,m.media_type,m.file_url,m.storage_path,m.original_file_url,m.original_storage_path,m.processed_file_url,m.processed_storage_path,m.processing_status,m.processing_error FROM hg_youban_publish_media m WHERE m.account_id=ANY(%s) AND m.deleted_at IS NULL ORDER BY m.profile_id,m.purpose,m.sort_index,m.id"""
    cur.execute(sql,(ids,)); rows=cur.fetchall()
    if args.limit: rows=rows[:args.limit]
    hosts=[]
    for label,key in [('img', 'YOUBAN_MEDIA_PUBLIC_BASE_URL'),('oss', 'YOUBAN_OSS_PUBLIC_BASE_URL'),('cos-fallback','YOUBAN_PUBLISH_MEDIA_CACHE_FALLBACK_CDN_BASE_URL')]:
        value=env(key).strip()
        if value: hosts.append((label,value))
    if not any(label=='img' for label,_ in hosts): hosts.append(('img','https://img.xiaohuiji.cc'))
    session=requests.Session(); session.headers['User-Agent']='youban-media-audit/1.0'
    report={'account':args.account,'accounts':[{'id':r[0],'tenantId':r[1],'nickname':r[2],'username':r[3]} for r in accounts], 'scanned':len(rows),'hosts':[h for _,h in hosts], 'items':[], 'summary':{}}
    for row in rows:
        (mid,aid,pid,purpose,mt,fu,sp,ofu,osp,pfu,psp,processing,perr)=row
        candidates=build_url_candidates(fu or ofu, sp or osp, hosts)
        probes=[]
        for label,url in candidates:
            probes.append({'source':label,'url':url,'probe':probe(session,url,args.timeout)})
        usable=next((x for x in probes if 200 <= x['probe']['status'] < 300),None)
        report['items'].append({'mediaId':mid,'accountId':aid,'profileId':pid,'purpose':purpose,'mediaType':mt,'fileUrl':fu,'storagePath':sp,'processingStatus':processing,'processingError':perr,'usable':usable,'probes':probes})
    summary={'total':len(rows),'usable':sum(1 for x in report['items'] if x['usable']), 'unusable':sum(1 for x in report['items'] if not x['usable']), 'processingFailed':sum(1 for x in report['items'] if x['processingStatus']=='failed')}
    summary['byOriginalHost']={}
    for x in report['items']:
        host=urlparse(x['fileUrl'] or '').netloc or '(empty)'; summary['byOriginalHost'][host]=summary['byOriginalHost'].get(host,0)+1
    report['summary']=summary
    with open(args.output,'w') as f: json.dump(report,f,ensure_ascii=False,indent=2)
    print(json.dumps({'output':args.output,'summary':summary},ensure_ascii=False))

if __name__=='__main__': main()
