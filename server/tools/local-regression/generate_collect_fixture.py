#!/usr/bin/env python3
"""生成本地采集回放夹具，不调用线上服务。

输出 SQL 可导入测试 PostgreSQL；事件由真实 collect-process/media/publish 角色处理。
"""
import argparse
from pathlib import Path


def q(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def main() -> None:
    parser = argparse.ArgumentParser(description="生成 BOT/账户/历史采集回放 SQL")
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--count", type=int, default=1000)
    parser.add_argument("--mode", choices=("bot", "account", "history"), required=True)
    parser.add_argument("--tenant-id", type=int, default=990)
    parser.add_argument("--account-id", type=int, default=0)
    parser.add_argument("--source-id", type=int, default=990001)
    parser.add_argument("--source-chat-id", default="-1009900000990")
    parser.add_argument("--rule-id", type=int, default=990001)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.count <= 0 or args.count > 10000:
        raise SystemExit("count 必须在 1 到 10000 之间")
    account_id = 0 if args.mode == "bot" else args.account_id
    history = "TRUE" if args.mode == "history" else "FALSE"
    source_type = "'bot'" if args.mode == "bot" else "'account'"
    rows = []
    for index in range(1, args.count + 1):
        event_id = args.source_id * 100000 + index
        rows.append(
            f"({event_id},{args.tenant_id},{account_id},{args.source_id},{source_type},1,0,"
            f"'{args.source_chat_id}',{index},{q(f'{args.run_id}:{index}')},"
            f"'display',0,'ready','回放资料 {args.run_id} {index}',1,'pending',NOW(),NOW(),NOW())"
        )
    values = ",\n".join(rows)
    sql = f"""-- 本地采集回放夹具 run_id={args.run_id} mode={args.mode}
BEGIN;
DELETE FROM hg_youban_publish_collect_process_outbox WHERE event_id BETWEEN {args.source_id * 100000 + 1} AND {args.source_id * 100000 + args.count};
DELETE FROM hg_youban_publish_collect_event WHERE id BETWEEN {args.source_id * 100000 + 1} AND {args.source_id * 100000 + args.count};
INSERT INTO hg_youban_publish_collect_event
(id,tenant_id,account_id,source_id,source_type,bot_id,tg_account_id,source_chat_id,source_message_id,source_unique_key,material_role,material_parent_event_id,material_group_status,raw_text,media_count,status,received_at,created_at,updated_at)
VALUES
{values};
INSERT INTO hg_youban_publish_collect_process_outbox
(event_id,tenant_id,account_id,source_id,source_chat_id,status,available_at,created_at,updated_at,history)
SELECT id,tenant_id,account_id,source_id,source_chat_id,'pending',NOW(),NOW(),NOW(),{history}
FROM hg_youban_publish_collect_event WHERE id BETWEEN {args.source_id * 100000 + 1} AND {args.source_id * 100000 + args.count};
COMMIT;
"""
    args.output.write_text(sql, encoding="utf-8")


if __name__ == "__main__":
    main()
