package system

// Panelden tetiklenen uzun-süren bakım işlerinin (güncelleme, optimizasyon, ...)
// canlı log çıktısı, panelin kendi varsayılan diline (panel_ayarlari.varsayilan_dil)
// göre gösterilir. Bu paketin route handler'ları stateless tasarlandığı için (DB
// parametresi almazlar), bağlantı burada package-level tutulur — main.go Init'i
// çağırır (bkz. system.SurumBaslat ile aynı desen).

import "database/sql"

var db *sql.DB

// Init: main.go panel DB bağlantısını burada set eder.
func Init(d *sql.DB) { db = d }

// panelDili: DB'ye erişilemezse (henüz Init edilmemiş, bağlantı sorunu) 'tr'ye düşer.
func panelDili() string {
	if db == nil {
		return "tr"
	}
	var dil string
	_ = db.QueryRow(`SELECT varsayilan_dil FROM panel_ayarlari WHERE id=1`).Scan(&dil)
	if dil != "en" {
		return "tr"
	}
	return "en"
}

// t: panelDili()'ne göre tr ya da en metnini döndürür.
func t(tr, en string) string {
	if panelDili() == "en" {
		return en
	}
	return tr
}
