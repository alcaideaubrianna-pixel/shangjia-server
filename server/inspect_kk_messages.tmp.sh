#!/usr/bin/env bash
set -e
export PGPASSWORD='password_T56ZMY'
PSQL="psql -h 10.5.4.5 -p 5432 -U user_maSrfW -d sj-db -P pager=off"
echo '=== channel ==='
$PSQL -c "select id,tenant_id,account_id,channel_title,channel_username,target_chat_id,status,deleted_at from hg_youban_publish_channel where lower(channel_username)='kkbaoyang01' or target_chat_id like '%kkbaoyang01%';"
echo '=== jobs ==='
$PSQL -c "select j.id,j.tenant_id,j.account_id,j.channel_id,j.profile_id,j.status,j.tg_message_id,j.collect_event_id,j.collect_source_id,j.collect_source_chat_id,j.collect_source_message_id,j.error_message,j.created_at,j.sent_at from hg_youban_publish_tg_job j join hg_youban_publish_channel c on c.id=j.channel_id where lower(c.channel_username)='kkbaoyang01' and j.tg_message_id in (22902,20781) order by j.tg_message_id;"
echo '=== tg message records ==='
$PSQL -c "select m.id,m.tenant_id,m.channel_id,m.profile_id,m.tg_message_id,m.media_group_id,m.created_at from hg_youban_publish_tg_message m join hg_youban_publish_channel c on c.id=m.channel_id where lower(c.channel_username)='kkbaoyang01' and m.tg_message_id in (22902,20781) order by m.tg_message_id,m.id;"
