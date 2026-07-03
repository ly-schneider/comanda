package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/qatestresult"
	"backend/internal/repository"
)

type fakeQaRepo struct {
	run            *ent.QaTestRun
	upsertedKey    string
	upsertedStatus *qatestresult.Status
	upsertedNote   *string
	getRunErr      error
}

func (f *fakeQaRepo) CreateRun(_ context.Context, name string, note *string) (*ent.QaTestRun, error) {
	f.run = &ent.QaTestRun{ID: "qa_test_____", Name: name, Note: note}
	return f.run, nil
}
func (f *fakeQaRepo) ListRuns(context.Context) ([]*ent.QaTestRun, error) { return nil, nil }
func (f *fakeQaRepo) GetRun(context.Context, string) (*ent.QaTestRun, error) {
	if f.getRunErr != nil {
		return nil, f.getRunErr
	}
	if f.run == nil {
		f.run = &ent.QaTestRun{ID: "qa_test_____"}
	}
	return f.run, nil
}
func (f *fakeQaRepo) UpdateRun(_ context.Context, _ string, name, note *string) (*ent.QaTestRun, error) {
	if f.run == nil {
		f.run = &ent.QaTestRun{ID: "qa_test_____"}
	}
	if name != nil {
		f.run.Name = *name
	}
	f.run.Note = note
	return f.run, nil
}
func (f *fakeQaRepo) DeleteRun(context.Context, string) error { return nil }
func (f *fakeQaRepo) ListResults(context.Context, string) ([]*ent.QaTestResult, error) {
	return nil, nil
}
func (f *fakeQaRepo) CountsByRun(context.Context) (map[string]repository.QaRunCounts, error) {
	return nil, nil
}
func (f *fakeQaRepo) UpsertResult(_ context.Context, _, testKey string, status *qatestresult.Status, note *string) (*ent.QaTestResult, error) {
	f.upsertedKey = testKey
	f.upsertedStatus = status
	f.upsertedNote = note
	return &ent.QaTestResult{TestKey: testKey, Status: status, Note: note}, nil
}

func TestQaCreateRunValidation(t *testing.T) {
	tests := []struct {
		name    string
		runName string
		note    *string
		wantErr error
	}{
		{"valid", "Staging pre-launch", nil, nil},
		{"trimmed to empty", "   ", nil, ErrInvalidRunName},
		{"too long", strings.Repeat("x", 121), nil, ErrInvalidRunName},
		{"note too long", "ok", ptrStr(strings.Repeat("n", qaNoteMaxLen+1)), ErrInvalidQaNote},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewQaService(&fakeQaRepo{})
			_, err := svc.CreateRun(context.Background(), tc.runName, tc.note)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CreateRun(%q) err = %v, want %v", tc.runName, err, tc.wantErr)
			}
		})
	}
}

func TestQaSetResultValidation(t *testing.T) {
	valid := "success"
	invalid := "meh"
	tests := []struct {
		name    string
		testKey string
		status  *string
		wantErr error
	}{
		{"valid with status", "shop.browse-grid", &valid, nil},
		{"valid without status", "shop.browse-grid", nil, nil},
		{"invalid status", "shop.browse-grid", &invalid, ErrInvalidQaStatus},
		{"empty key", "", &valid, ErrInvalidTestKey},
		{"uppercase key", "Shop.Browse", &valid, ErrInvalidTestKey},
		{"key too long", strings.Repeat("k", 121), &valid, ErrInvalidTestKey},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeQaRepo{}
			svc := NewQaService(repo)
			_, err := svc.SetResult(context.Background(), "run1", tc.testKey, tc.status, nil)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("SetResult(%q) err = %v, want %v", tc.testKey, err, tc.wantErr)
			}
		})
	}
}

func TestQaSetResultUnknownRun(t *testing.T) {
	repo := &fakeQaRepo{getRunErr: repository.ErrNotFound}
	svc := NewQaService(repo)
	_, err := svc.SetResult(context.Background(), "missing", "shop.browse-grid", nil, nil)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("SetResult on missing run err = %v, want ErrNotFound", err)
	}
	if repo.upsertedKey != "" {
		t.Fatal("SetResult must not upsert when the run lookup fails")
	}
}

func ptrStr(s string) *string { return &s }
