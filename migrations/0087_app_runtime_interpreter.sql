ALTER TABLE app_runtimes ADD COLUMN interpreter VARCHAR(255) NOT NULL DEFAULT '' AFTER runtime;
