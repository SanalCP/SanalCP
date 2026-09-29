ALTER TABLE app_runtimes ADD COLUMN release_dir VARCHAR(255) NOT NULL DEFAULT '' AFTER health_path;
