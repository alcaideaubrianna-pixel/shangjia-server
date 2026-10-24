CREATE INDEX idx_ybbps_share_lookup ON hg_youban_bot_profile_session(bot_id,scene,profile_no,status,expires_at);
CREATE INDEX idx_ybbps_scene_expire ON hg_youban_bot_profile_session(scene,expires_at,id);
