import argparse, json, re, unicodedata

FEE = re.compile(r'(?:介绍|推荐|牵线|居间|对接)费')
NUM = re.compile(r'[0-9０-９]+')
INVISIBLE = re.compile(r'[\u200b-\u200f\u2060\ufeff\ufe00-\ufe0f]')
MULTI_MARKERS = ('国内', '国外', '村内', '村外', '一人', '双人', '打包', '单个', '/', '、')

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
    ap=argparse.ArgumentParser(); ap.add_argument('--apply',action='store_true'); ap.add_argument('--limit',type=int,default=10000); args=ap.parse_args()
    conn=psycopg2.connect(dbname='railway', user='postgres')
    cur=conn.cursor()
    cur.execute('''SELECT DISTINCT ON (d.profile_id) d.profile_id,p.profile_no,p.plain_text,COALESCE(o.suffix,i.value) AS suffix
      FROM hg_youban_publish_collect_dispatch d
      JOIN hg_youban_publish_collect_rule_item i ON i.rule_id=d.rule_id AND i.item_type='intro_fee_suffix'
      JOIN hg_content_profile p ON p.id=d.profile_id
      JOIN hg_youban_publish_collect_source s ON s.id=d.source_id
      LEFT JOIN LATERAL (SELECT r.suffix FROM hg_youban_publish_intro_fee_channel_rule r
        WHERE r.tenant_id=d.tenant_id AND r.rule_id IN (d.rule_id,0) AND r.account_id IN (d.account_id,0)
          AND r.status=1 AND r.deleted_at IS NULL AND r.channel_id IN (s.source_chat_id,regexp_replace(s.source_chat_id,'^-',''))
          AND btrim(r.suffix)<>'' ORDER BY (r.rule_id=d.rule_id) DESC,(r.account_id=d.account_id) DESC,r.id DESC LIMIT 1) o ON TRUE
      WHERE d.source_type_snapshot IN ('bot','account') AND p.deleted_at IS NULL AND btrim(COALESCE(o.suffix,i.value))<>''
      ORDER BY d.profile_id,d.id DESC LIMIT %s''',(args.limit,))
    rows=cur.fetchall(); changes=[]; skipped=[]
    for pid,pno,text,suffix in rows:
        lines=text.splitlines(); new=[]; changed=False
        for line in lines:
            out,did=format_line(line,suffix); new.append(out); changed |= did
        value='\n'.join(new)
        if '\\1' in value: raise RuntimeError(f'backref residue profile={pid}')
        if changed: changes.append((pid,pno,text,value,suffix))
    print(json.dumps({'scanned':len(rows),'candidates':len(changes),'dryRun':not args.apply,'profiles':[{'id':p[0],'profileNo':p[1],'suffix':p[4],'beforeFee':next((x for x in p[2].splitlines() if FEE.search(x)),''),'afterFee':next((x for x in p[3].splitlines() if FEE.search(x)),'')} for p in changes]},ensure_ascii=False))
    if args.apply:
        for pid,pno,before,after,suffix in changes:
            cur.execute('UPDATE hg_content_profile SET plain_text=%s, updated_at=NOW() WHERE id=%s AND plain_text=%s',(after,pid,before))
        conn.commit(); print(json.dumps({'applied':len(changes)},ensure_ascii=False))
    conn.close()
if __name__ == '__main__':
    main()
