#!/usr/bin/env python3
"""生成可由真实采集、媒体和发布 Worker 处理的隔离采集回放夹具。"""
import argparse
from pathlib import Path


def q(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def main() -> None:
    parser = argparse.ArgumentParser(description="生成 BOT/账户/历史采集 SQL 夹具")
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--count", type=int, default=1000)
    parser.add_argument("--mode", choices=("bot", "account", "history"), required=True)
    parser.add_argument("--tenant-id", type=int, default=990)
    parser.add_argument("--account-id", type=int, default=0)
    parser.add_argument("--source-id", type=int, default=990001)
    parser.add_argument("--source-chat-id", default="-1009900000990")
    parser.add_argument("--rule-id", type=int, default=990001)
    parser.add_argument("--channel-id", type=int, default=2)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.count <= 0 or args.count > 10000:
        raise SystemExit("count 必须在 1 到 10000 之间")
    if args.tenant_id <= 0 or args.source_id <= 0 or args.rule_id <= 0 or args.channel_id <= 0:
        raise SystemExit("tenant/source/rule/channel ID 必须为正数")
    account_id = 0 if args.mode == "bot" else args.account_id
    if args.mode != "bot" and account_id <= 0:
        raise SystemExit("account/history 模式必须提供正数 account-id")
    history = "TRUE" if args.mode == "history" else "FALSE"
    source_type = "bot" if args.mode == "bot" else "account"
    bot_id = 1 if args.mode == "bot" else 0
    tg_account_id = 0 if args.mode == "bot" else account_id
    first_event = args.source_id * 100000
    last_event = first_event + args.count
    events = []
    media = []
    for index in range(1, args.count + 1):
        event_id = first_event + index
        unique_key = q(f"{args.run_id}:{index}")
        text = q(f"回放资料 {args.run_id} {index}")
        events.append(
            f"({event_id},{args.tenant_id},{account_id},{args.source_id},{q(source_type)},{bot_id},{tg_account_id},"
            f"{q(args.source_chat_id)},{index},{unique_key},'display',0,'ready',{text},1,'pending',NOW(),NOW(),NOW())"
        )
        media.append(
            f"({args.tenant_id},{account_id},{args.source_id},{q(source_type)},{event_id},{q(args.source_chat_id)},"
            f"{index},{q(f'{args.run_id}:media:{index}')},'photo','bot','local-file-{args.run_id}-{index}',"
            f"'', 'local-replay-image.jpg','photo','image/jpeg',1047,0,'ready',1,NOW(),NOW())"
        )
    source_name = q(f"本地回放来源 {args.run_id}")
    rule_name = q(f"本地回放规则 {args.run_id}")
    event_sql = ",\n".join(events)
    media_sql = ",\n".join(media)
    sql = f"""-- 本地完整采集回放夹具 run_id={args.run_id} mode={args.mode} count={args.count}
BEGIN;
-- 先清理本 run 的事实记录，保证脚本可重复执行。
DELETE FROM hg_youban_publish_collect_process_outbox WHERE event_id BETWEEN {first_event + 1} AND {last_event};
DELETE FROM hg_youban_publish_collect_event_media WHERE event_id BETWEEN {first_event + 1} AND {last_event};
DELETE FROM hg_youban_publish_collect_event WHERE id BETWEEN {first_event + 1} AND {last_event};
INSERT INTO hg_youban_publish_collect_channel
(id,tenant_id,telegram_chat_id,chat_type,title,status,created_at,updated_at)
VALUES ({args.channel_id},{args.tenant_id},{q(args.source_chat_id)},'channel',{source_name},'active',NOW(),NOW())
ON CONFLICT (id) DO UPDATE SET status='active',updated_at=NOW();
INSERT INTO hg_youban_publish_collect_bot_channel
(tenant_id,bot_id,channel_id,status,bot_role,first_seen_at,last_seen_at,created_at,updated_at)
VALUES ({args.tenant_id},{bot_id},{args.channel_id},'active','collector',NOW(),NOW(),NOW(),NOW())
ON CONFLICT (tenant_id,bot_id,channel_id) DO UPDATE SET status='active',bot_role='collector',last_seen_at=NOW(),updated_at=NOW();
INSERT INTO hg_youban_publish_collect_account_channel
(tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,approved_at,created_at,updated_at)
VALUES ({args.tenant_id},{account_id},{args.channel_id},{args.source_id},1,'approved','approved',NOW(),NOW(),NOW())
ON CONFLICT (tenant_id,account_id,channel_id,source_id) DO UPDATE SET collect_enabled=1,status='approved',approval_status='approved',approved_at=NOW(),updated_at=NOW();
INSERT INTO hg_youban_publish_collect_rule
(id,tenant_id,account_id,name,global_enabled,review_enabled,dedupe_enabled,dedupe_days,
 block_link,block_username,block_plain_text,status,created_at,updated_at)
VALUES ({args.rule_id},{args.tenant_id},{account_id},{rule_name},0,0,0,7,0,0,0,1,NOW(),NOW())
ON CONFLICT (id) DO UPDATE SET status=1,updated_at=NOW();
INSERT INTO hg_youban_publish_collect_rule_item
(tenant_id,account_id,rule_id,item_type,value,replacement,sort,created_at)
VALUES ({args.tenant_id},{account_id},{args.rule_id},'keyword','回放资料','',1,NOW())
ON CONFLICT DO NOTHING;
INSERT INTO hg_youban_publish_collect_source
(id,tenant_id,account_id,source_type,title,source_chat_id,source_username,tg_account_id,bot_id,
 bot_collect_scope,collect_enabled,status,channel_id,channel_blacklisted,history_collect_enabled,
 history_collect_mode,history_collect_days,created_at,updated_at)
VALUES ({args.source_id},{args.tenant_id},{account_id},{q(source_type)},{source_name},{q(args.source_chat_id)},'',
 {tg_account_id},{bot_id},'chat',1,1,{args.channel_id},0,1,'recent_days',30,NOW(),NOW())
ON CONFLICT (id) DO UPDATE SET account_id=EXCLUDED.account_id,collect_enabled=1,status=1,channel_id=EXCLUDED.channel_id,updated_at=NOW();
INSERT INTO hg_youban_publish_collect_source_rule
(tenant_id,source_id,rule_id,sort,status,created_at,updated_at)
VALUES ({args.tenant_id},{args.source_id},{args.rule_id},1,1,NOW(),NOW())
ON CONFLICT (source_id,rule_id) DO UPDATE SET status=1,updated_at=NOW();
INSERT INTO hg_youban_publish_collect_rule_channel
(tenant_id,account_id,rule_id,channel_id,created_at)
VALUES ({args.tenant_id},{account_id},{args.rule_id},{args.channel_id},NOW())
ON CONFLICT (rule_id,channel_id) DO NOTHING;
INSERT INTO hg_youban_publish_collect_event
(id,tenant_id,account_id,source_id,source_type,bot_id,tg_account_id,source_chat_id,source_message_id,source_unique_key,
 material_role,material_parent_event_id,material_group_status,raw_text,media_count,status,received_at,created_at,updated_at)
VALUES
{event_sql};
INSERT INTO hg_youban_publish_collect_event_media
(tenant_id,account_id,source_id,source_type,event_id,source_chat_id,source_message_id,source_media_key,media_type,
 source_ref_type,source_file_id,file_url,storage_path,source_kind,source_mime_type,source_size,sort_index,cache_status,cache_hit,created_at,updated_at)
VALUES
{media_sql};
INSERT INTO hg_youban_publish_collect_process_outbox
(event_id,tenant_id,account_id,source_id,source_chat_id,status,available_at,created_at,updated_at,history)
SELECT id,tenant_id,account_id,source_id,source_chat_id,'pending',NOW(),NOW(),NOW(),{history}
FROM hg_youban_publish_collect_event WHERE id BETWEEN {first_event + 1} AND {last_event};
COMMIT;
"""
    args.output.write_text(sql, encoding="utf-8")


if __name__ == "__main__":
    main()
