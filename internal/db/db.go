package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func Open(dsn string) (*sql.DB, error) {
	d, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	maxOpen := havuzBoyutu()
	d.SetMaxOpenConns(maxOpen)
	d.SetMaxIdleConns(maxOpen / 2)
	d.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return d, nil
}

// havuzBoyutu — açık bağlantı üst sınırı.
//
// Sabit 16, tek çekirdekli küçük bir VPS için doğru ama 50+ domainli kurulumda
// darboğaz olur: DB içe/dışa aktarma uçları httpx.ExtendDeadline ile bir
// bağlantıyı 10 dakikaya kadar tutabildiği için birkaç paralel iş havuzu
// tüketip normal panel isteklerini bloklayabilir. Çekirdek sayısına göre
// ölçekle, tabanı 16'da tut (eski davranış), SANALCP_DB_MAX_CONNS ile ez.
func havuzBoyutu() int {
	if v := strings.TrimSpace(os.Getenv("SANALCP_DB_MAX_CONNS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 2 && n <= 512 {
			return n
		}
		log.Printf("SANALCP_DB_MAX_CONNS geçersiz (%q), otomatik değer kullanılacak", v)
	}
	n := runtime.NumCPU() * 4
	if n < 16 {
		n = 16
	}
	if n > 128 {
		n = 128 // MariaDB max_connections'ı tek başına tüketme
	}
	return n
}
