import argparse, json, os, re, unicodedata

FEE = re.compile(r'(?:介绍|推荐|牵线|居间|对接|感谢)费')
NUM = re.compile(r'[0-9０-９]+')
INVISIBLE = re.compile(r'[\u200b-\u200f\u2060\ufeff\ufe00-\ufe0f]')
MULTI_MARKERS = ('国内', '国外', '村内', '村外', '一人', '双人', '打包', '单个', '、')

def norm(s):
    return ''.join(ch for ch in unicodedata.normalize('NFKC', s) if not (unicodedata.category(ch) == 'Cf' or ch in '\ufe00\ufe0f'))

def format_line(line, suffix):
    clean = norm(line).strip()
    if not FEE.search(clean): return line, False
    nums = NUM.findall(clean)
    foreign = re.search(r'国外[^0-9０-９]*([0-9０-９]+)', clean)
    if foreign and len(nums) >= 2:
        out = f'介绍费 {nums[0]} 国外 {foreign.group(1)} {suffix}'
    elif len(nums) > 1 and any(marker in clean for marker in MULTI_MARKERS):
        out = clean if clean.endswith(suffix) else clean.rstrip() + ' ' + suffix
    elif nums:
        out = f'介绍费 {nums[0]} {suffix}'
    else:
        out = clean if clean.endswith(suffix) else clean.rstrip() + ' ' + suffix
    return out, out != line

def main():
    import psycopg2
    ap=argparse.ArgumentParser(description='Generate fee-repair cases from a database snapshot. Writes are disabled by design.')
    ap.add_argument('--apply',action='store_true', help=argparse.SUPPRESS)
    ap.add_argument('--missing-only',action='store_true'); ap.add_argument('--today-only',action='store_true'); ap.add_argument('--limit',type=int,default=10000)
    args=ap.parse_args()
    if args.apply:
        raise SystemExit('refusing production writes: export the preview and review it with an approved migration tool')
    dsn = os.environ.get('DATABASE_URL') or os.environ.get('PG_DSN')
    if not dsn:
        raise SystemExit('DATABASE_URL or PG_DSN is required; refusing local default connection')
    conn=psycopg2.connect(dsn)
    cur=conn.cursor()
    extra = " AND p.plain_text !~ '(介绍|推荐|牵线|居间|对接|感谢)费' AND e.raw_text ~ '(介绍|推荐|牵线|居间|对接|感谢)费'" if args.missing_only else ""
    if args.today_only:
        extra += " AND e.created_at >= CURRENT_DATE"
    cur.execute('''SELECT DISTINCT ON (d.profile_id) d.profile_id,p.profile_no,p.plain_text,COALESCE(o.suffix,NULLIF(btrim(s.channel_suffix),''),i.value) AS suffix
       FROM hg_youban_publish_collect_dispatch d
      LEFT JOIN hg_youban_publish_collect_rule_item i ON i.rule_id=d.rule_id AND i.item_type='intro_fee_suffix'
      JOIN hg_content_profile p ON p.id=d.profile_id
      JOIN hg_youban_publish_collect_event e ON e.id=d.event_id
      JOIN hg_youban_publish_collect_source s ON s.id=d.source_id
      LEFT JOIN LATERAL (SELECT r.suffix FROM hg_youban_publish_intro_fee_channel_rule r
        WHERE r.tenant_id=d.tenant_id AND r.rule_id IN (d.rule_id,0) AND r.account_id IN (d.account_id,0)
          AND r.bot_id::text IN (s.bot_id::text,'0') AND r.status=1 AND r.deleted_at IS NULL
          AND r.channel_id::text IN (s.channel_id::text,'0')
          AND btrim(r.suffix)<>'' ORDER BY (r.channel_id::text=s.channel_id::text) DESC,(r.bot_id::text=s.bot_id::text) DESC,(r.account_id=d.account_id) DESC,(r.rule_id=d.rule_id) DESC,r.id DESC LIMIT 1) o ON TRUE
      WHERE d.source_type_snapshot IN ('bot','account') AND p.deleted_at IS NULL AND btrim(COALESCE(o.suffix,NULLIF(btrim(s.channel_suffix),''),i.value))<>''
      ''' + extra + ''' ORDER BY d.profile_id,d.id DESC LIMIT %s''',(args.limit,))
    rows=cur.fetchall(); changes=[]; skipped=[]
    for pid,pno,text,suffix in rows:
        # Repair the current stored profile text. Historical event text may
        # contain an older "original | transformed" pair and must not be used
        # as the optimistic-lock value for the current row.
        lines=text.splitlines(); new=[]; changed=False
        for line in lines:
            out,did=format_line(line,suffix); new.append(out); changed |= did
        value='\n'.join(new)
        if '\\1' in value: raise RuntimeError(f'backref residue profile={pid}')
        if changed: changes.append((pid,pno,text,value,suffix))
    print(json.dumps({'scanned':len(rows),'candidates':len(changes),'dryRun':True,'profiles':[{'id':p[0],'profileNo':p[1],'suffix':p[4],'beforeFee':next((x for x in p[2].splitlines() if FEE.search(x)),''),'afterFee':next((x for x in p[3].splitlines() if FEE.search(x)),'')} for p in changes]},ensure_ascii=False))
    conn.close()
if __name__ == '__main__':
    main()
