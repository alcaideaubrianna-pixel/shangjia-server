#!/usr/bin/env python3
"""Offline strict temporal classification of ID-only loop audit evidence."""
import argparse, collections, datetime, json, os, pathlib

def stamp(value):
    return datetime.datetime.fromisoformat(value) if value else None

def norm(value):
    s=str(value or '').strip()
    return s[4:] if s.startswith('-100') and s[4:].isdigit() else s

def classify(manifest, output, exclusions=()):
    os.umask(0o077)
    output=pathlib.Path(output); output.mkdir(parents=True,exist_ok=False,mode=0o700)
    rows=[json.loads(s) for s in pathlib.Path(manifest).read_text().splitlines()]
    excluded={834927}
    for path in exclusions:
        text=pathlib.Path(path).read_text()
        old=json.loads(text) if text.lstrip().startswith('[') else [json.loads(s) for s in text.splitlines()]
        for r in old:
            if isinstance(r,int):excluded.add(r)
            elif 'profileIds' in r:excluded.update(r['profileIds'])
            else:excluded.add(r['profile_id'])
    groups=collections.defaultdict(dict); timed=[]; mixed=[]; other=[]; confirmed=[]
    for r in rows:
        e=r.get('event');o=r.get('origin');when=stamp((e or {}).get('created_at'))
        eligible=[c for c in r['target_evidence'] if c['tenant_id']==r['tenant_id'] and norm(c['target_chat_id'])==norm((e or {}).get('source_chat_id')) and when and stamp(c.get('created_at')) and stamp(c['created_at'])<=when and (not c.get('deleted_at') or stamp(c['deleted_at'])>=when)]
        r['time_evidence']={'rule':'target.created_at <= event.created_at AND (target.deleted_at IS NULL OR target.deleted_at >= event.created_at)','event_created_at':(e or {}).get('created_at'),'qualifying_targets':eligible}
        if not eligible:
            r['classification']='timeUncertain';timed.append(r);continue
        if r['manual_review_mixed_origin'] or len(r['all_origins'])!=1:
            r['classification']='mixedOrMultipleOrigins';mixed.append(r);continue
        valid=e and o and o['origin_kind']=='collect' and o['evidence_kind']=='event' and all(e[k]==r[k] for k in ('tenant_id','account_id','source_id')) and e['source_message_id']==o['message_id'] and o['event_id']==e['id'] and norm(o['chat_id'])==norm(e['source_chat_id']) and norm(r['actual_source_chat'])==norm(e['source_chat_id']) and all(o[k]==r[k] for k in ('tenant_id','account_id','source_id','profile_id'))
        if not valid:
            r['classification']='otherEvidenceUncertain';other.append(r);continue
        r['classification']='timeVerified';r['excluded_already_handed_off']=r['profile_id'] in excluded
        confirmed.append(r)
        if r['profile_id'] not in excluded:groups[(r['tenant_id'],r['account_id'])][r['profile_id']]=r
    ids={}
    for (t,a),rs in sorted(groups.items()):
        ordered=sorted(rs);ids[f'{t}/{a}']=ordered
        for offset in range(0,len(ordered),50):
            (output/f'safe-tenant{t}-account{a}-batch{offset//50+1:03d}.json').write_text(json.dumps([rs[i] for i in ordered[offset:offset+50]],ensure_ascii=False))
    for name,data in [('timeUncertain.json',timed),('mixedOrMultipleOrigins.json',mixed),('otherEvidenceUncertain.json',other),('timeVerified-all.json',confirmed),('safe-scope-ids.json',ids)]:
        (output/name).write_text(json.dumps(data,ensure_ascii=False,indent=2))
    summary={'input':str(pathlib.Path(manifest).resolve()),'evidence_rows':len(rows),'unique_profiles':len({r['profile_id'] for r in rows}),'time_verified_unique':len({r['profile_id'] for r in confirmed}),'timeUncertain_unique':len({r['profile_id'] for r in timed}),'mixedOrMultipleOrigins_unique':len({r['profile_id'] for r in mixed}),'otherEvidenceUncertain_unique':len({r['profile_id'] for r in other}),'safe_excluding_handoffs':sum(map(len,ids.values())),'safe_scope_counts':{k:len(v) for k,v in ids.items()},'exclusions_unique':len(excluded),'note':'Read-only evidence snapshot; existence and Telegram deletion are not asserted at classification time.'}
    (output/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2))
    for f in output.iterdir():os.chmod(f,0o600)
    return summary

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('manifest');p.add_argument('output');p.add_argument('--exclude-manifest',action='append',default=[]);a=p.parse_args();print(json.dumps(classify(a.manifest,a.output,a.exclude_manifest),ensure_ascii=False))
