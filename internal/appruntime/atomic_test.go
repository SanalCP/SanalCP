package appruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const newCommit = "2222222222222222222222222222222222222222"

func TestAtomicReleaseHealthFailureRestoresPreviousUnit(t *testing.T) {
	var steps []string
	hooks := releaseHooks{
		prepare:  func(context.Context) (string, error) { steps = append(steps, "prepare"); return "build", nil },
		switchTo: func(context.Context) error { steps = append(steps, "switch"); return nil },
		health:   func(context.Context) error { steps = append(steps, "health"); return errors.New("HTTP 500") },
		persist:  func(context.Context) error { t.Fatal("failed release persisted"); return nil },
		restore:  func(context.Context) error { steps = append(steps, "restore"); return nil },
		discard:  func(_ context.Context, rel string) error { steps = append(steps, "discard:"+rel); return nil },
	}
	staged := StagedRelease{Commit: newCommit, ReleaseDir: "new", Log: "git"}
	result, err := runReleaseTransition(context.Background(), staged, "old", hooks)
	if err == nil || !strings.Contains(err.Error(), "geri yüklendi") {
		t.Fatalf("rollback error: %v", err)
	}
	if !result.RolledBack || result.Commit != "" {
		t.Fatalf("result: %+v", result)
	}
	if strings.Join(steps, ",") != "prepare,switch,health,restore,discard:new" {
		t.Fatalf("steps: %v", steps)
	}
}

func TestAtomicReleaseSuccessPersistsBeforeOldCleanup(t *testing.T) {
	var steps []string
	hooks := releaseHooks{
		prepare:  func(context.Context) (string, error) { steps = append(steps, "prepare"); return "", nil },
		switchTo: func(context.Context) error { steps = append(steps, "switch"); return nil },
		health:   func(context.Context) error { steps = append(steps, "health"); return nil },
		persist:  func(context.Context) error { steps = append(steps, "persist"); return nil },
		restore:  func(context.Context) error { t.Fatal("unexpected rollback"); return nil },
		discard:  func(_ context.Context, rel string) error { steps = append(steps, "discard:"+rel); return nil },
	}
	result, err := runReleaseTransition(context.Background(), StagedRelease{Commit: newCommit, ReleaseDir: "new"}, "old", hooks)
	if err != nil || result.Commit != newCommit {
		t.Fatalf("result=%+v, err=%v", result, err)
	}
	if strings.Join(steps, ",") != "prepare,switch,health,persist,discard:old" {
		t.Fatalf("steps: %v", steps)
	}
}

func TestAtomicReleaseFailedRestoreKeepsStage(t *testing.T) {
	discarded := false
	hooks := releaseHooks{
		prepare:  func(context.Context) (string, error) { return "", nil },
		switchTo: func(context.Context) error { return errors.New("switch failed") },
		health:   func(context.Context) error { return nil },
		persist:  func(context.Context) error { return nil },
		restore:  func(context.Context) error { return errors.New("restore failed") },
		discard:  func(context.Context, string) error { discarded = true; return nil },
	}
	result, err := runReleaseTransition(context.Background(), StagedRelease{Commit: newCommit, ReleaseDir: "new"}, "old", hooks)
	if err == nil || !strings.Contains(err.Error(), "dönüş başarısız") {
		t.Fatalf("error: %v", err)
	}
	if result.RolledBack || discarded {
		t.Fatalf("failed restore discarded staged release: %+v", result)
	}
}
