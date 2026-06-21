UPDATE `hg_content_media`
SET `display_storage_path` = `original_storage_path`,
    `updated_at` = NOW()
WHERE `media_type` = 'video'
  AND `deleted_at` IS NULL
  AND IFNULL(`original_storage_path`, '') <> ''
  AND IFNULL(`display_storage_path`, '') <> `original_storage_path`;
