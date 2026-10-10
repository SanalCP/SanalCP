// MySQL string/identifier kaçış yardımcıları.
//
// CREATE USER / ALTER USER / GRANT gibi DDL ifadelerinde `?` placeholder
// çalışmaz; o yüzden literal kaçışı doğru yapılmalıdır. `sqlKac` yalnız
// backslash ve tek tırnak kaçırıyordu — NUL, satır sonu ve Ctrl+Z (0x1a)
// gibi MySQL için anlamlı karakterleri ıskalıyordu.
//
// Kural: identifier için MySQLIdent (GecerliDBKimlik'ten GEÇMİŞ olmalı),
// string literal için MySQLStringLiteral. İkisi de tırnak/backtick dahil
// döndürür; çağıran taraf ek tırnak EKLEMEZ.

package hesaplar

import "strings"

// MySQLStringLiteral: s değerini tek-tırnaklı MySQL string literal'ine
// çevirir (tırnaklar dahil). Kaçırılan karakterler: NUL, LF, CR, Ctrl+Z,
// backslash, tek tırnak, çift tırnak.
//
// Ayrıntı: https://dev.mysql.com/doc/refman/8.0/en/string-literals.html
func MySQLStringLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case 0:
			b.WriteString(`\0`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case 0x1a: // Ctrl+Z — Windows EOF, MySQL `\Z` eşdeğeri
			b.WriteString(`\Z`)
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// MySQLIdent: s değerini backtick'li MySQL identifier'ına çevirir
// (backtick'ler dahil). S YALNIZCA GecerliDBKimlik'ten geçmiş identifier
// için kullanılmalıdır; bu fonksiyon ek savunma katmanı olarak backtick
// kaçırır, identifier sözdizimini doğrulamaz.
func MySQLIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// MySQLUserHost: 'user'@'host' çiftini güvenli literal olarak üretir.
// Hem user hem host MySQLStringLiteral'dan geçirilir.
func MySQLUserHost(user, host string) string {
	return MySQLStringLiteral(user) + "@" + MySQLStringLiteral(host)
}
