package plans

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// PUT gövdesinde mailbox_quota_mb yoksa mevcut değer KORUNMALI (nil → SQL
// COALESCE); 0 ise açıkça "sınırsız"a çekmektir. Frontend bugün tam plan
// nesnesi yollar, ama eski istemciler/betikler alanı hiç bilmez.
func TestPlanMailboxQuotaOmittedPreservesOnUpdate(t *testing.T) {
	_, _, omitted, err := decodePlanReq(httptest.NewRequest(http.MethodPut, "/plans/1", strings.NewReader(`{"ad":"p"}`)), false)
	if err != nil {
		t.Fatal(err)
	}
	if omitted != nil {
		t.Fatalf("alan yokken COALESCE argümanı nil olmalı, geldi: %v", omitted)
	}

	p, _, explicit, err := decodePlanReq(httptest.NewRequest(http.MethodPut, "/plans/1", strings.NewReader(`{"ad":"p","mailbox_quota_mb":0}`)), false)
	if err != nil {
		t.Fatal(err)
	}
	if explicit != 0 || p.MailboxQuotaMB != 0 {
		t.Fatalf("açık 0 korunmalı (sınırsız): arg=%v plan=%d", explicit, p.MailboxQuotaMB)
	}

	p, _, set, err := decodePlanReq(httptest.NewRequest(http.MethodPut, "/plans/1", strings.NewReader(`{"ad":"p","mailbox_quota_mb":2048}`)), false)
	if err != nil || set != 2048 || p.MailboxQuotaMB != 2048 {
		t.Fatalf("değer okunmadı: arg=%v plan=%d err=%v", set, p.MailboxQuotaMB, err)
	}
}

// Sınır dışı değer DB'ye gitmeden 400 almalı (Handlers{} nil DB: dokunulursa panik).
func TestPlanMailboxQuotaSinirDisiReddedilir(t *testing.T) {
	h := &Handlers{}
	for _, body := range []string{
		`{"ad":"p","mailbox_quota_mb":-1}`,
		`{"ad":"p","mailbox_quota_mb":1048577}`,
	} {
		for _, handler := range []http.HandlerFunc{h.Create, h.Update} {
			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(body)))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s → %d %s, 400 bekleniyordu", body, rec.Code, rec.Body.String())
			}
		}
	}
}
