-- 0049 — S3 uyumlu uzak yedek hedefleri (Amazon S3 / Backblaze B2).
ALTER TABLE backup_destinations ADD COLUMN IF NOT EXISTS bucket     varchar(253) NOT NULL DEFAULT '';
ALTER TABLE backup_destinations ADD COLUMN IF NOT EXISTS region     varchar(64)  NOT NULL DEFAULT 'us-east-1';
ALTER TABLE backup_destinations ADD COLUMN IF NOT EXISTS endpoint   varchar(512) NOT NULL DEFAULT '';
ALTER TABLE backup_destinations ADD COLUMN IF NOT EXISTS path_style tinyint(4)   NOT NULL DEFAULT 1;
