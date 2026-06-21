-- 公告移动端展示扩展字段。
-- PostgreSQL version. 可重复执行：字段和索引存在时不会重复创建。

ALTER TABLE hg_admin_notice
  ADD COLUMN IF NOT EXISTS is_banner smallint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS banner_img varchar(500),
  ADD COLUMN IF NOT EXISTS banner_url varchar(500),
  ADD COLUMN IF NOT EXISTS publish_at timestamp,
  ADD COLUMN IF NOT EXISTS expire_at timestamp;

COMMENT ON COLUMN hg_admin_notice.is_banner IS '是否Banner';
COMMENT ON COLUMN hg_admin_notice.banner_img IS 'Banner图片';
COMMENT ON COLUMN hg_admin_notice.banner_url IS 'Banner链接';
COMMENT ON COLUMN hg_admin_notice.publish_at IS '定时发布时间';
COMMENT ON COLUMN hg_admin_notice.expire_at IS '过期时间';

CREATE INDEX IF NOT EXISTS idx_admin_notice_public
  ON hg_admin_notice (type, status, publish_at, expire_at, sort, id);

CREATE INDEX IF NOT EXISTS idx_admin_notice_banner
  ON hg_admin_notice (is_banner, status, publish_at, expire_at, sort, id);
