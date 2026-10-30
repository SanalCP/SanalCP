package backups

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

func failingBackupCommands(t *testing.T, command string) {
	t.Helper()
	dir := t.TempDir()
	commands := map[string]string{
		"mysql":     "#!/bin/sh\nexit 0\n",
		"mysqldump": "#!/bin/sh\nprintf '%s' '-- MySQL dump'\nexit 0\n",
		"tar":       "#!/bin/sh\nprintf partial > \"$2\"\nexit 0\n",
	}
	commands[command] = strings.TrimSuffix(commands[command], "exit 0\n") + "exit 7\n"
	for name, script := range commands {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestCreateArchiveCommandFailureCleansPartial(t *testing.T) {
	for _, command := range []string{"mysqldump", "tar"} {
		t.Run(command, func(t *testing.T) {
			failingBackupCommands(t, command)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectQuery(`SELECT DISTINCT db_name`).WithArgs(int64(7)).
				WillReturnRows(sqlmock.NewRows([]string{"db_name"}).AddRow("tenant_main"))
			dir := t.TempDir()
			if _, err := createArchive(context.Background(), db, 7, "example.test", "c_test", filepath.Join(dir, "backup.tar.gz")); err == nil || !strings.Contains(err.Error(), command+":") && !strings.Contains(err.Error(), command+" ") {
				t.Fatalf("expected %s failure, got %v", command, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("partial archive remains: %v, %v", entries, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestArchiveFailurePreservesExistingRecoveryPoint(t *testing.T) {
	failingBackupCommands(t, "tar")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT DISTINCT db_name`).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"db_name"}))
	dir := t.TempDir()
	archive := filepath.Join(dir, "existing.tar.gz")
	if err := os.WriteFile(archive, []byte("last good archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := createArchive(context.Background(), db, 7, "example.test", "c_test", archive); err == nil {
		t.Fatal("tar failure accepted")
	}
	data, err := os.ReadFile(archive)
	if err != nil || string(data) != "last good archive" {
		t.Fatalf("existing recovery point overwritten: %q, %v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("partial archive remains: %v, %v", entries, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBackupFlowsRejectCommandFailure(t *testing.T) {
	for _, flow := range []string{"manual", "automatic", "bulk"} {
		for _, command := range []string{"mysqldump", "tar"} {
			t.Run(flow+"/"+command, func(t *testing.T) {
				failingBackupCommands(t, command)
				sk := fmt.Sprintf("c_test%x", time.Now().UnixNano())
				dir := filepath.Join(BackupRoot, sk)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Skipf("backup root is not writable: %v", err)
				}
				defer os.RemoveAll(dir)
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if flow == "manual" {
					mock.ExpectQuery(`SELECT alan_adi, sistem_kullanici, is_demo`).WithArgs(int64(7)).
						WillReturnRows(sqlmock.NewRows([]string{"alan_adi", "sistem_kullanici", "is_demo"}).AddRow("example.test", sk, 0))
				}
				mock.ExpectQuery(`SELECT DISTINCT db_name`).WithArgs(int64(7)).
					WillReturnRows(sqlmock.NewRows([]string{"db_name"}).AddRow("tenant_main"))
				d := dueDomain{ID: 7, AlanAdi: "example.test", SK: sk, Retention: 1}
				switch flow {
				case "manual":
					r := httptest.NewRequest(http.MethodPost, "/", nil)
					route := chi.NewRouteContext()
					route.URLParams.Add("id", "7")
					r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, route))
					w := httptest.NewRecorder()
					(&Handlers{DB: db}).Create(w, r)
					if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), command) {
						t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
					}
				case "automatic":
					if err := runOneBackup(db, d, "test"); err == nil || !strings.Contains(err.Error(), command) {
						t.Fatalf("command failure was not propagated: %v", err)
					}
				case "bulk":
					mock.ExpectQuery(`SELECT COALESCE\(MAX\(id\),0\)`).WithArgs(int64(7)).
						WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(0))
					mock.ExpectQuery(`SELECT id, dosya, uzak_durum FROM backups`).WithArgs(int64(7), TipOto).
						WillReturnRows(sqlmock.NewRows([]string{"id", "dosya", "uzak_durum"}))
					topluMu.Lock()
					topluDur = TopluDurum{Calisiyor: true, Toplam: 1}
					topluMu.Unlock()
					topluCalistir(db, []dueDomain{d})
					topluMu.Lock()
					got := topluDur
					topluMu.Unlock()
					if got.Tamamlanan != 0 || got.Basarisiz != 1 || len(got.Hatalar) != 1 || !strings.Contains(got.Hatalar[0], command) {
						t.Fatalf("failed archive counted as success: %+v", got)
					}
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 0 {
					t.Fatalf("partial archive remains: %v, %v", entries, err)
				}
				// No INSERT or last_backup_at UPDATE is expected on any failed path.
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestCountRetentionProtectsLastVerified(t *testing.T) {
	for _, tip := range []string{TipOto, TipManuel} {
		for _, state := range []string{"bekliyor", "basarisiz"} {
			t.Run(tip+"/"+state, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				mock.ExpectQuery(`SELECT COALESCE\(MAX\(id\),0\)`).WithArgs(int64(7)).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
				mock.ExpectQuery(`SELECT id, dosya, uzak_durum FROM backups`).WithArgs(int64(7), tip).
					WillReturnRows(sqlmock.NewRows([]string{"id", "dosya", "uzak_durum"}).
						AddRow(13, state+".tar.gz", "").AddRow(12, "older-unverified.tar.gz", "").AddRow(11, "verified.tar.gz", "").AddRow(10, "old.tar.gz", ""))
				for _, id := range []int64{10, 12} {
					mock.ExpectExec(`DELETE FROM backups WHERE id=\?`).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
				}
				var pruneErr error
				if tip == TipOto {
					pruneErr = pruneOld(db, 7, "c_missing", 1)
				} else {
					pruneErr = pruneManuel(db, 7, "c_missing", 1)
				}
				if pruneErr != nil {
					t.Fatal(pruneErr)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestAgeRetentionProtectsLastVerified(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(id\),0\)`).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
	mock.ExpectQuery(`SELECT id, dosya, uzak_durum FROM backups`).WithArgs(int64(7), TipOto, 3, int64(7), TipOto).
		WillReturnRows(sqlmock.NewRows([]string{"id", "dosya", "uzak_durum"}).AddRow(10, "old.tar.gz", "").AddRow(11, "verified.tar.gz", "").AddRow(12, "failed.tar.gz", ""))
	for _, id := range []int64{10, 12} {
		mock.ExpectExec(`DELETE FROM backups WHERE id=\?`).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	if err := pruneEski(db, 7, "c_missing", 3); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManualUnlimitedRetentionUnchanged(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := pruneManuel(db, 7, "c_missing", 0); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBulkCleanupProtectsLastVerified(t *testing.T) {
	for _, mod := range []string{ModOto, ModManuel, ModGun} {
		for _, state := range []string{"bekliyor", "dogrulaniyor", "basarisiz"} {
			t.Run(mod+"/"+state, func(t *testing.T) {
				tip := TipOto
				if mod == ModManuel {
					tip = TipManuel
				}
				esik := time.Now().AddDate(0, 0, -3)
				bilinen := map[string]dbKayit{
					"verified.tar.gz": {ID: 11, Tip: tip, DogrulamaDurum: "dogrulandi", Olusturma: esik.AddDate(0, 0, -7).Unix()},
					"new.tar.gz":      {ID: 12, Tip: tip, DogrulamaDurum: state, Olusturma: esik.AddDate(0, 0, -1).Unix()},
					"older.tar.gz":    {ID: 10, Tip: tip, DogrulamaDurum: "dogrulandi", Olusturma: esik.AddDate(0, 0, -9).Unix()},
				}
				adaylar, err := temizlikAdaylari(7, "c_missing", mod, esik, bilinen)
				if err != nil || len(adaylar) != 2 {
					t.Fatalf("unexpected cleanup candidates: %+v, %v", adaylar, err)
				}
				for _, a := range adaylar {
					if a.BackupID == 11 {
						t.Fatal("last verified recovery point selected for deletion")
					}
				}
			})
		}
	}
}
