CREATE TABLE IF NOT EXISTS islem_merkezi_gizlenenler (
  anahtar VARCHAR(191) NOT NULL PRIMARY KEY,
  gizlendi_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY ix_islem_merkezi_gizlendi_at (gizlendi_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
