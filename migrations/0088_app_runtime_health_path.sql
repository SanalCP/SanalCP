ALTER TABLE app_runtimes ADD COLUMN health_path VARCHAR(128) NOT NULL DEFAULT '/' AFTER entrypoint;
