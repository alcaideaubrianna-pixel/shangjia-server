#!/usr/bin/env python3
"""Read-only source351 business pending/media facts. Does not modify raw exports."""
import argparse
import collections
import datetime
import hashlib
import json
import os
import pathlib
import queue
import re
import subprocess
import threading
import time

STATUSES=('pending','group_collecting','waiting_order','prechecked','media_pending','media_ready','failed')
EVENT='hg_youban_publish_collect_event'
MEDIA='hg_youban_publish_collect_event_media'

def now():
    return datetime.datetime.now().astimezone().isoformat()

def normalize(chat):
    s=str(chat).strip()
    if s.startswith('-100') and s[4:].isdigit():return s[4:]
    if s.isdigit() and int(s)>0:return s
    raise ValueError('Invalid approved numeric channel ID')

def quote(value):
    return "'"+str(value).replace("'","''")+"'"

def camel(name):
    first,*rest=name.split('_')
    return first+''.join(part.title() for part in rest)

class Session:
    def __init__(self, directory):
        self.last_sql=None
        self.stage='connect'
        self.events=queue.Queue()
        self.errors=(directory/'business-psql-stderr.log').open('x')
        self.plans=(directory/'business-query-plans.jsonl').open('x')
        self.proc=subprocess.Popen(['ssh','-i','/Users/ley/.ssh/id_hk_rsa','-o','BatchMode=yes','-o','ControlMaster=no','-o','ControlPath=none','-o','ConnectTimeout=10','-o','ServerAliveInterval=10','-o','ServerAliveCountMax=2','root@47.236.85.171',"sudo -u postgres env LC_ALL=C PGAPPNAME=collect_business_backlog_readonly PGOPTIONS='-c default_transaction_read_only=on -c statement_timeout=3000 -c lock_timeout=500 -c max_parallel_workers_per_gather=0' stdbuf -oL psql -X -qAt -v ON_ERROR_STOP=0 -d railway"],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=self.errors,text=True,bufsize=1)
        def reader():
            try:
                for line in self.proc.stdout:self.events.put(line.rstrip('\n'))
            finally:self.events.put(None)
        threading.Thread(target=reader,daemon=True).start()
    def raw(self,sql):
        self.last_sql=sql
        self.proc.stdin.write(sql+';\n\\if :ERROR\n\\echo __BUSINESS_SQL_ERROR__ :SQLSTATE\n\\endif\n\\echo __BUSINESS_EXPORT_END__\n');self.proc.stdin.flush()
        lines=[];sql_error=None
        while True:
            try:line=self.events.get(timeout=60)
            except queue.Empty:raise RuntimeError('No response for 60s; no retry')
            if line is None:raise RuntimeError('psql/SSH ended; see business-psql-stderr.log; no retry')
            if line.startswith('__BUSINESS_SQL_ERROR__'):
                sql_error=line;continue
            if line=='__BUSINESS_EXPORT_END__':
                if sql_error:raise RuntimeError(sql_error+'; no retry')
                return lines
            lines.append(line)
    def query(self,sql,kind):
        wrapped='SELECT row_to_json(facts) FROM ('+sql+') facts'
        plan=json.loads(''.join(self.raw('EXPLAIN (FORMAT JSON) '+wrapped)))[0]['Plan']
        self.plans.write(json.dumps({'time':now(),'stage':self.stage,'kind':kind,'sql':sql,'plan':plan})+'\n');self.plans.flush()
        if plan.get('Total Cost',0)>10000:raise RuntimeError('Plan cost exceeds 10000; not executed')
        def validate(node):
            if node.get('Node Type') in ('Seq Scan','Parallel Seq Scan'):raise RuntimeError('Sequential scan rejected; not executed')
            relation=node.get('Relation Name');condition=node.get('Index Cond','')
            if relation==EVENT and node.get('Node Type') in ('Index Scan','Index Only Scan','Bitmap Index Scan'):
                if 'source_id' not in condition or (kind=='event' and 'source_chat_id' not in condition):raise RuntimeError('Event plan lacks bounded source/chat index condition')
            if relation==MEDIA and node.get('Node Type') in ('Index Scan','Index Only Scan','Bitmap Index Scan') and 'event_id' not in condition:raise RuntimeError('Media plan lacks event_id index condition')
            for child in node.get('Plans',[]):validate(child)
        if kind!='catalog':validate(plan)
        if kind=='highwater':
            def ordered(node):
                return (node.get('Index Name')=='idx_ybp_collect_event_order' and node.get('Scan Direction')=='Backward') or any(ordered(c) for c in node.get('Plans',[]))
            if not ordered(plan):raise RuntimeError('Highwater plan is not reverse idx_ybp_collect_event_order; scope skipped')
        return [json.loads(line) for line in self.raw(wrapped)]
    def close(self):
        if self.proc.poll() is None:
            try:self.proc.stdin.close();self.proc.wait(timeout=10)
            except (BrokenPipeError,subprocess.TimeoutExpired):self.proc.terminate();self.proc.wait(timeout=10)
        self.plans.close();self.errors.close()

def main():
    p=argparse.ArgumentParser();p.add_argument('--directory',default='tmp/collector351-backlog-20261118-v3-business');p.add_argument('--approval',default='tmp/collector351-backlog-20261118-v2/summary.json');a=p.parse_args()
    os.umask(0o077);directory=pathlib.Path(a.directory);directory.mkdir(parents=True,exist_ok=True)
    original=pathlib.Path(a.approval);snapshot=original.read_bytes();approved=json.loads(snapshot)
    if (approved['tenantId'],approved['accountId'],approved['sourceId'])!=(78,615,351):raise ValueError('Approval scope mismatch')
    chats=list(dict.fromkeys(normalize(c['chatId']) for c in approved['channels']))
    blocked={normalize(c) for c in approved.get('excludedPublishChats',[])}
    if blocked.intersection(chats):raise ValueError('Approved list overlaps excluded publish targets')
    scopes=[{'account':account,'chat':variant} for chat in chats for variant in (chat,'-100'+chat) for account in (615,0)]
    summary={'tenantId':78,'sourceId':351,'storageAccounts':[615,0],'approvedChannels':len(chats),'chatAccountScopes':len(scopes),'approvalSummarySHA256':hashlib.sha256(snapshot).hexdigest(),'startedAt':now(),'statuses':STATUSES,'processedAtMustBeNull':True,'businessEvents':0,'businessMedia':0,'completedScopes':0,'failures':[],'highwaters':[],'scope':'Business collect_event pending states only, distinct from original raw received/failed_retry export','concurrency':1,'statementTimeoutMs':3000,'snapshot':'Fixed per-chat/account message_id/id highwaters; per-statement read committed, not a transaction-wide snapshot. Late arrivals below the tuple highwater may be observed.'}
    for name in ('business-pending.jsonl','business-media.jsonl','business-summary.json','business-query-plans.jsonl','business-psql-stderr.log','business-schema.json'):
        if (directory/name).exists():raise FileExistsError('Refusing to overwrite existing business artifact: '+name)
    pending=(directory/'business-pending.jsonl').open('x');media=(directory/'business-media.jsonl').open('x')
    session=None;cursor=None;seen=set();media_seen=set();status_counts=collections.Counter();account_counts=collections.Counter()
    try:
        session=Session(directory);session.stage='schema'
        schema=session.query(f"SELECT table_name,column_name,data_type FROM information_schema.columns WHERE table_schema='public' AND table_name IN ({quote(EVENT)},{quote(MEDIA)}) ORDER BY table_name,ordinal_position",'catalog')
        indexes=session.query(f"SELECT tablename,indexname,indexdef FROM pg_indexes WHERE schemaname='public' AND tablename IN ({quote(EVENT)},{quote(MEDIA)})",'catalog')
        (directory/'business-schema.json').write_text(json.dumps({'columns':schema,'indexes':indexes},indent=2))
        columns={table:[r['column_name'] for r in schema if r['table_name']==table] for table in (EVENT,MEDIA)}
        if not all(columns.values()):raise RuntimeError('Business schema missing')
        def fields(table):return ','.join('"'+c+'" AS "'+camel(c)+'"' for c in columns[table])
        statuses=','.join(quote(s) for s in STATUSES)
        def where(scope):return f"tenant_id=78 AND source_id=351 AND account_id={scope['account']} AND source_chat_id={quote(scope['chat'])} AND processed_at IS NULL AND status IN ({statuses})"
        session.stage='per-chat/account reverse indexed highwaters'
        for number,scope in enumerate(scopes):
            cursor={'accountId':scope['account'],'chatId':scope['chat'],'phase':'highwater'}
            scope['highwater']=None
            try:
                prefix=f"tenant_id=78 AND source_id=351 AND account_id={scope['account']} AND source_chat_id={quote(scope['chat'])}"
                high=session.query(f'SELECT source_message_id,id FROM {EVENT} WHERE {prefix} ORDER BY source_message_id DESC LIMIT 1','highwater')
                if high:
                    message=high[0]['source_message_id']
                    tie=session.query(f'SELECT id FROM {EVENT} WHERE {prefix} AND source_message_id={message} ORDER BY id DESC LIMIT 1','event')
                    scope['highwater']=[message,tie[0]['id']]
            except Exception as error:
                scope['skipped']=True
                summary['failures'].append({'error':str(error),'stage':'highwater','cursor':cursor,'lastSQL':session.last_sql,'retry':False})
            summary['highwaters'].append(dict(scope))
            print('highwater scope',number+1,'/',len(scopes),'skipped',len(summary['failures']),flush=True)
        summary['highwatersCapturedAt']=now()
        for scope in scopes:
            if scope.get('skipped'):continue
            try:
                cursor={'accountId':scope['account'],'chatId':scope['chat'],'lastMessageId':None,'lastId':None,'highwater':scope['highwater'],'mediaEventIds':None}
                high=scope['highwater']
                if high is None:summary['completedScopes']+=1;continue
                last=None
                while True:
                    session.stage='event page'
                    bounds=f' AND (source_message_id,id)<=({high[0]},{high[1]})'
                    if last is not None:bounds+=f' AND (source_message_id,id)>({last[0]},{last[1]})'
                    rows=session.query(f'SELECT {fields(EVENT)} FROM {EVENT} WHERE {where(scope)}{bounds} ORDER BY source_message_id,id LIMIT 50','event')
                    if not rows:break
                    for row in rows:
                        if row['id'] in seen:raise RuntimeError('Duplicate event encountered')
                        if row['tenantId']!=78 or row['sourceId']!=351 or row['accountId'] not in (615,0) or normalize(row['sourceChatId']) not in chats or row['status'] not in STATUSES or row['processedAt'] is not None:raise RuntimeError('Unexpected event outside approval')
                        seen.add(row['id']);pending.write(json.dumps(row,ensure_ascii=False)+'\n');status_counts[row['status']]+=1;account_counts[row['accountId']]+=1;summary['businessEvents']+=1
                    pending.flush()
                    ids=[row['id'] for row in rows];cursor['mediaEventIds']=ids
                    session.stage='event media indexed page'
                    media_cursor=None
                    while True:
                        bounds='' if media_cursor is None else f' AND (event_id,sort_index,id)>({media_cursor[0]},{media_cursor[1]},{media_cursor[2]})'
                        media_rows=session.query(f"SELECT {fields(MEDIA)} FROM {MEDIA} WHERE event_id IN ({','.join(map(str,ids))}){bounds} ORDER BY event_id,sort_index,id LIMIT 200",'media')
                        for row in media_rows:
                            if row['id'] in media_seen or row['eventId'] not in ids:raise RuntimeError('Unexpected/duplicate media row')
                            media_seen.add(row['id']);media.write(json.dumps(row,ensure_ascii=False)+'\n');summary['businessMedia']+=1
                        media.flush()
                        if len(media_rows)<200:break
                        row=media_rows[-1];media_cursor=(row['eventId'],row['sortIndex'],row['id']);cursor['mediaCursor']=media_cursor
                    last=(rows[-1]['sourceMessageId'],rows[-1]['id']);cursor.update(lastMessageId=last[0],lastId=last[1],mediaEventIds=None)
                    print('business events',summary['businessEvents'],'media',summary['businessMedia'],'completed scopes',summary['completedScopes'],flush=True)
                    if len(rows)<50:break
                summary['completedScopes']+=1
            except Exception as error:
                summary['failures'].append({'error':str(error),'stage':session.stage,'cursor':cursor,'lastSQL':session.last_sql,'retry':False})
                print('scope skipped after failure',scope['account'],scope['chat'],flush=True)
    except Exception as error:
        summary['failures'].append({'error':str(error),'stage':session.stage if session else 'startup','cursor':cursor,'lastSQL':session.last_sql if session else None,'retry':False,'remainingScopes':scopes[summary['completedScopes']:]})
        print('Export stopped; failure/cursor saved in business-summary.json',flush=True)
    finally:
        pending.close();media.close()
        if session:session.close()
        summary.update(finishedAt=now(),statusCounts=dict(status_counts),storageAccountCounts=dict(account_counts),uniqueBusinessEvents=len(seen),uniqueBusinessMedia=len(media_seen),complete=not summary['failures'] and summary['completedScopes']==len(scopes),rawApprovalUnchanged=original.read_bytes()==snapshot)
        (directory/'business-summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2))
        for name in ('business-pending.jsonl','business-media.jsonl','business-summary.json','business-query-plans.jsonl','business-psql-stderr.log','business-schema.json'):
            if (directory/name).exists():os.chmod(directory/name,0o600)
        print(json.dumps({k:summary[k] for k in ('businessEvents','businessMedia','completedScopes','chatAccountScopes','complete','rawApprovalUnchanged','failures')},ensure_ascii=False),flush=True)
    return 0 if summary['complete'] else 1

if __name__=='__main__':raise SystemExit(main())
