-- 0055 — mail_send_log retention temizliği için ts indeksi.
-- Mevcut indeksler (mailbox_id, ts) ve (domain_id, ts) bileşiktir; yalnız ts'e
-- göre yapılan periyodik DELETE bunları kullanamaz ve tabloyu baştan tarardı.
ALTER TABLE mail_send_log ADD INDEX IF NOT EXISTS ix_sendlog_ts (ts);
