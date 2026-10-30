-- Legacy cron backups belong to the automatic retention pool.
-- Preserve archives, verification metadata and all existing domain settings.
UPDATE backups SET tip='oto' WHERE tip='planli';
