#!/usr/bin/env python3
"""Read-only production ID evidence export; rerun creates a new run directory."""
import argparse, collections, datetime, json, os, pathlib, re, subprocess, sys
os.umask(0o077)
p=argparse.ArgumentParser(); p.add_argument('--dispatch-source',type=int,action='append'); p.add_argument('--dispatch-only',action='store_true'); p.add_argument('--exclude-manifest',action='append',default=[]); p.add_argument('--priority-only',action='store_true'); p.add_argument('--output',default='tmp/collect-loop-audit-20261118'); a=p.parse_args()
out=pathlib.Path(a.output); out.mkdir(parents=True,exist_ok=True); os.chmod(out,0o700)
run=out/datetime.datetime.now().strftime('run-%Y%m%d-%H%M%S'); run.mkdir(mode=0o700)
log=(run/'queries.jsonl').open('w'); manifest=(run/'manifest.jsonl').open('w'); failures=[]; seen=set(); counts=collections.Counter(); origin_cache={}; last_sql=None
ssh=subprocess.Popen(['ssh','-i','/Users/ley/.ssh/id_hk_rsa','-o','BatchMode=yes','-o','ConnectTimeout=10','-o','ServerAliveInterval=10','-o','ServerAliveCountMax=2','root@47.236.85.171',"sudo -u postgres env LC_ALL=C PGAPPNAME=collect_loop_readonly_audit PGOPTIONS='-c default_transaction_read_only=on -c statement_timeout=3000 -c lock_timeout=500 -c max_parallel_workers_per_gather=0 -c application_name=collect_loop_readonly_audit' stdbuf -oL psql -X -qAt -v ON_ERROR_STOP=1 -d railway"],stdin=subprocess.PIPE,stdout=subprocess.PIPE,text=True,bufsize=1)
def raw(sql):
    ssh.stdin.write(sql+';\n\\echo __AUDIT_END__\n'); ssh.stdin.flush(); lines=[]
    while True:
        s=ssh.stdout.readline()
        if not s: raise RuntimeError('psql stopped (timeout/error); see stderr')
        if s.strip()=='__AUDIT_END__': return lines
        lines.append(s.rstrip())
def query(sql,small=False):
    global last_sql
    last_sql=sql
    match=re.search(r'\bIN \(([0-9,]+)\)',sql)
    if match and len(match.group(1).split(','))>500:
        ids=match.group(1).split(','); result=[]
        for offset in range(0,len(ids),500):
            part=sql[:match.start(1)]+','.join(ids[offset:offset+500])+sql[match.end(1):]
            result.extend(query(part,small))
        return result
    plan=json.loads(''.join(raw('EXPLAIN (FORMAT JSON) '+sql)))[0]['Plan']
    log.write(json.dumps({'sql':sql,'plan':plan})+'\n'); log.flush()
    def check(n):
        if not small and (n.get('Node Type')=='Seq Scan'): raise RuntimeError('unsafe plan: '+str(n))
        for c in n.get('Plans',[]): check(c)
    if not small and plan.get('Total Cost',0)>20000: raise RuntimeError('unsafe total plan cost')
    check(plan)
    return [json.loads(s) for s in raw('SELECT row_to_json(a) FROM ('+sql+') a')]
def lit(x): return "'"+str(x).replace("'","''")+"'"
def norm(x):
    s=str(x or '').strip()
    if s.startswith('-100') and s[4:].isdigit(): return s[4:]
    return s if s.isdigit() and int(s)>0 else None
def emit(r,kind):
    ev=r.pop('event',None); origin=r.pop('origin',None)
    chat=(ev or {}).get('source_chat_id') if ev else (origin or {}).get('chat_id')
    key=(int(r['tenant_id']),norm(chat)); evidence=targets.get(key,[])
    if not evidence: return
    if ev and any(int(ev[k])!=int(r[k]) for k in ('tenant_id','account_id','source_id')):
        failures.append({'kind':'event_scope_mismatch','event_id':ev['id'],'profile_id':r['profile_id']}); return
    if origin and ev and (norm(origin['chat_id'])!=norm(chat) or int(origin['message_id'] or 0)!=int(ev['source_message_id'] or 0)):
        failures.append({'kind':'origin_event_mismatch','origin_id':origin['id']}); return
    ident=(kind,r['profile_id'],(ev or {}).get('id'),(origin or {}).get('id'))
    if ident in seen:return
    seen.add(ident)
    all_origins=origin_cache[r['profile_id']]
    mixed=any(o['origin_kind']!='collect' or (int(o['tenant_id']),norm(o['chat_id'])) not in targets for o in all_origins)
    r.update(all_origins=all_origins,manual_review_mixed_origin=mixed,evidence_kind=kind,event=ev,origin=origin,actual_source_chat=chat,target_evidence=evidence,target_scope='active' if any(x['deleted_at'] is None for x in evidence) else 'history',excluded_already_handled=r['profile_id']==834927)
    manifest.write(json.dumps(r,ensure_ascii=False)+'\n'); manifest.flush(); counts[kind]+=1
try:
    channels=query('SELECT id,tenant_id,target_chat_id,publish_direction,status,deleted_at,created_at FROM hg_youban_publish_channel',True)
    sources=query('SELECT id,tenant_id,account_id,source_chat_id,deleted_at FROM hg_youban_publish_collect_source',True)
    accounts=query('SELECT id,tenant_id FROM hg_youban_publish_account',True)
    targets=collections.defaultdict(list)
    for c in channels:
        if c['publish_direction']=='up' and c['status']==1 and norm(c['target_chat_id']): targets[(c['tenant_id'],norm(c['target_chat_id']))].append(c)
    (run/'targets.json').write_text(json.dumps(channels)); (run/'sources.json').write_text(json.dumps(sources))
    # Batch 24 independent indexed scope reads per round trip, still one connection/query.
    scopes={(s['tenant_id'],s['account_id']) for s in sources}|{(x['tenant_id'],x['id']) for x in accounts}
    jobs=[]
    for tenant,account in sorted(scopes,key=lambda x:(x!=(78,615),x)):
        if a.priority_only and (tenant,account)!=(78,615):continue
        for t,chat in targets:
            if t==tenant:
                for variant in (chat,'-100'+chat):jobs.append([tenant,account,variant,0,0])
    if a.dispatch_only:jobs=[]
    (run/'scope-inventory.json').write_text(json.dumps(jobs))
    while jobs:
        batch=jobs[:24];jobs=jobs[24:]; parts=[]
        for tenant,account,chat,cursor,oid in batch:
            parts.append(f"(SELECT id,tenant_id,account_id,profile_id,source_id,event_id,chat_id,chat_id_raw,message_id,origin_kind,evidence_kind,created_at FROM hg_youban_publish_profile_origin WHERE tenant_id={tenant} AND account_id={account} AND chat_id={lit(chat)} AND profile_id>={cursor} AND (profile_id,id)>({cursor},{oid}) ORDER BY profile_id,id LIMIT 500)")
        rows=query(' UNION ALL '.join(parts))
        for tenant,account,chat,cursor,oid in batch:
            rs=[r for r in rows if (r['tenant_id'],r['account_id'],r['chat_id'])==(tenant,account,chat)]
            if len(rs)==500:
                last=rs[-1]; jobs.append([tenant,account,chat,last['profile_id'],last['id']])
            else:counts['scopes_complete']+=1
        if rows:
            ids=','.join(map(str,{r['profile_id'] for r in rows}))
            profiles={r['id']:r for r in query(f'SELECT id,profile_no,created_at FROM hg_content_profile WHERE id IN ({ids}) AND deleted_at IS NULL')}
            existing=','.join(map(str,profiles))
            if existing:
                origins=query(f'SELECT id,profile_id,tenant_id,account_id,source_id,event_id,chat_id,message_id,origin_kind,evidence_kind FROM hg_youban_publish_profile_origin WHERE profile_id IN ({existing})')
                for pid in profiles:origin_cache[pid]=[]
                for o in origins:origin_cache[o['profile_id']].append(o)
            eids=','.join(str(r['event_id']) for r in rows if r['event_id'] and r['profile_id'] in profiles)
            events={r['id']:r for r in query(f'SELECT id,tenant_id,account_id,source_id,source_chat_id,source_message_id,status,created_at FROM hg_youban_publish_collect_event WHERE id IN ({eids})')} if eids else {}
            for o in rows:
                if o['profile_id'] not in profiles:continue
                if o['origin_kind']!='collect':
                    failures.append({'non_collect_origin_id':o['id']});continue
                prof=profiles[o['profile_id']]
                emit({'tenant_id':o['tenant_id'],'account_id':o['account_id'],'source_id':o['source_id'],'profile_id':o['profile_id'],'profile_no':prof['profile_no'],'profile_created_at':prof['created_at'],'event':events.get(o['event_id']),'origin':o},'origin_exact_chat')
        print('scopes complete',counts['scopes_complete'],'pending',len(jobs),'evidence',len(seen),flush=True)
    if a.dispatch_only:
        for source in sorted(sources,key=lambda r:(r['id']!=351,r['id'])):
            if a.dispatch_source and source['id'] not in a.dispatch_source:continue
            if not any(t==source['tenant_id'] for t,c in targets):continue
            sid=source['id']; cursor=(0,0,0)
            while True:
                pid,eid,did=cursor
                ds=query(f'SELECT id AS dispatch_id,tenant_id,account_id,source_id,event_id,profile_id FROM hg_youban_publish_collect_dispatch WHERE source_id={sid} AND profile_id>={pid} AND (profile_id,event_id,id)>({pid},{eid},{did}) AND profile_id>0 ORDER BY profile_id,event_id,id LIMIT 500')
                if not ds:break
                last=ds[-1];cursor=(last['profile_id'],last['event_id'],last['dispatch_id'])
                ids=','.join(map(str,{d['profile_id'] for d in ds}))
                profiles={r['id']:r for r in query(f'SELECT id,profile_no,created_at FROM hg_content_profile WHERE id IN ({ids}) AND deleted_at IS NULL')}
                ds=[d for d in ds if d['profile_id'] in profiles]
                if not ds:continue
                ids=','.join(map(str,{d['event_id'] for d in ds if d['event_id']}))
                events={r['id']:r for r in query(f'SELECT id,tenant_id,account_id,source_id,source_chat_id,source_message_id,status,created_at FROM hg_youban_publish_collect_event WHERE id IN ({ids})')} if ids else {}
                matched=[d for d in ds if d['event_id'] in events and (d['tenant_id'],norm(events[d['event_id']]['source_chat_id'])) in targets]
                if not matched:continue
                ids=','.join(map(str,{d['profile_id'] for d in matched}))
                origins=query(f'SELECT id,profile_id,tenant_id,account_id,source_id,event_id,chat_id,message_id,origin_kind,evidence_kind FROM hg_youban_publish_profile_origin WHERE profile_id IN ({ids})')
                for d in matched:origin_cache[d['profile_id']]=[]
                for o in origins:origin_cache[o['profile_id']].append(o)
                for d in matched:
                    e=events[d['event_id']];prof=profiles[d['profile_id']]
                    osame=[o for o in origin_cache[d['profile_id']] if o['event_id']==e['id'] and norm(o['chat_id'])==norm(e['source_chat_id']) and o['message_id']==e['source_message_id']]
                    d.update(event=e,origin=osame[0] if osame else None,profile_no=prof['profile_no'],profile_created_at=prof['created_at'])
                    emit(d,'event_dispatch')
            counts['dispatch_sources_complete']+=1
            print('dispatch source complete',sid,'sources',counts['dispatch_sources_complete'],'evidence',len(seen),flush=True)
    (run/'handled-sample.json').write_text(json.dumps({'profile_id':834927,'profile_no':'F30261','event_id':16246132,'source_id':351,'tenant_id':78,'account_id':615,'chat_id':'-1004429787816','message_id':425215,'excluded_already_handled':True,'handling_evidence':'User confirms physical deletion completed and 80 TG messages deleted; not independently revalidated here.'}))

except Exception as e:
    failures.append({'fatal':str(e),'last_sql':last_sql}); print(str(e),file=sys.stderr)
finally:
    if ssh.poll() is None:ssh.stdin.close(); ssh.wait(timeout=10)
    manifest.close(); log.close()
    records=[json.loads(s) for s in (run/'manifest.jsonl').read_text().splitlines()]
    excluded={834927}
    for path in a.exclude_manifest:
        text=pathlib.Path(path).read_text()
        previous=json.loads(text) if text.lstrip().startswith('[') else [json.loads(line) for line in text.splitlines()]
        excluded.update(r['profile_id'] for r in previous)
    groups=collections.defaultdict(dict); review=[]; safe=[]
    for r in records:
        e=r.get('event');o=r.get('origin')
        when=(e or {}).get('created_at')
        time_ok=bool(when and any(c.get('created_at') and c['created_at']<=when and (not c.get('deleted_at') or c['deleted_at']>=when) for c in r['target_evidence']))
        confirmed=(time_ok and len(r['all_origins'])==1 and e and o and o['origin_kind']=='collect' and o['evidence_kind']=='event' and all(e[k]==r[k] for k in ('tenant_id','account_id','source_id')) and e['source_message_id']==o['message_id'] and r['all_origins'] and not r['manual_review_mixed_origin'])
        r['excluded_already_handed_off']=r['profile_id'] in excluded
        if not confirmed:review.append(r);continue
        safe.append(r)
        if r['profile_id'] not in excluded:groups[(r['tenant_id'],r['account_id'])][r['profile_id']]=r
    (run/'all-safe.json').write_text(json.dumps(safe,ensure_ascii=False))
    (run/'manual-review.json').write_text(json.dumps(review,ensure_ascii=False))
    scope_ids={}
    for (tenant,account),byid in groups.items():
        ids=sorted(byid);scope_ids[f'{tenant}/{account}']=ids
        for offset in range(0,len(ids),50):
            (run/f'safe-tenant{tenant}-account{account}-batch{offset//50+1:03d}.json').write_text(json.dumps([byid[i] for i in ids[offset:offset+50]],ensure_ascii=False))
    (run/'new-safe-scope-ids.json').write_text(json.dumps(scope_ids,indent=2))
    from collect_loop_audit_classify import classify
    strict=classify(run/'manifest.jsonl',run/'time-verified',a.exclude_manifest)
    summary={'strict_temporal_classification':strict,'run':str(run.resolve()),'counts':dict(counts),'safe_unique_profiles':len({r['profile_id'] for r in safe}),'review_rows':len(review),'new_safe_unique_profiles':sum(len(v) for v in scope_ids.values()),'excluded_handoff_profiles':sorted(excluded),'evidence_rows':len(records),'unique_profiles':len({r['profile_id'] for r in records}),'failures':failures,'limitations':['Current channel configuration proves exact same-tenant target chat, not historical configuration at event time.','No content-similarity matching. Missing origin records cannot be recovered by this origin-index audit; no full event scan performed.','Read committed per statement; concurrent production changes may alter results.'],'finished_at':datetime.datetime.now().isoformat()}
    (run/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)); print(json.dumps(summary,ensure_ascii=False),flush=True)
    for f in run.iterdir():os.chmod(f,0o600)
    if failures:sys.exit(1)
