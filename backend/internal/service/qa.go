package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/qatestresult"
	"backend/internal/repository"
)

var (
	ErrInvalidRunName  = errors.New("invalid_run_name")
	ErrInvalidQaNote   = errors.New("invalid_note")
	ErrInvalidTestKey  = errors.New("invalid_test_key")
	ErrInvalidQaStatus = errors.New("invalid_qa_status")
)

// testKeyPattern mirrors the key format of web/lib/qa-tests.ts.
var testKeyPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

const qaNoteMaxLen = 10_000

type QaService interface {
	CreateRun(ctx context.Context, name string, note *string) (*ent.QaTestRun, error)
	ListRuns(ctx context.Context) ([]*ent.QaTestRun, map[string]repository.QaRunCounts, error)
	GetRun(ctx context.Context, id string) (*ent.QaTestRun, []*ent.QaTestResult, error)
	UpdateRun(ctx context.Context, id string, name, note *string) (*ent.QaTestRun, error)
	DeleteRun(ctx context.Context, id string) error
	SetResult(ctx context.Context, runID, testKey string, status, note *string) (*ent.QaTestResult, error)
}

type qaService struct {
	qa repository.QaRepository
}

func NewQaService(qa repository.QaRepository) QaService {
	return &qaService{qa: qa}
}

func validateRunName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return "", ErrInvalidRunName
	}
	return name, nil
}

func validateQaNote(note *string) error {
	if note != nil && len(*note) > qaNoteMaxLen {
		return ErrInvalidQaNote
	}
	return nil
}

func (s *qaService) CreateRun(ctx context.Context, name string, note *string) (*ent.QaTestRun, error) {
	name, err := validateRunName(name)
	if err != nil {
		return nil, err
	}
	if err := validateQaNote(note); err != nil {
		return nil, err
	}
	return s.qa.CreateRun(ctx, name, note)
}

func (s *qaService) ListRuns(ctx context.Context) ([]*ent.QaTestRun, map[string]repository.QaRunCounts, error) {
	runs, err := s.qa.ListRuns(ctx)
	if err != nil {
		return nil, nil, err
	}
	counts, err := s.qa.CountsByRun(ctx)
	if err != nil {
		return nil, nil, err
	}
	return runs, counts, nil
}

func (s *qaService) GetRun(ctx context.Context, id string) (*ent.QaTestRun, []*ent.QaTestResult, error) {
	run, err := s.qa.GetRun(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	results, err := s.qa.ListResults(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return run, results, nil
}

func (s *qaService) UpdateRun(ctx context.Context, id string, name, note *string) (*ent.QaTestRun, error) {
	if name != nil {
		validated, err := validateRunName(*name)
		if err != nil {
			return nil, err
		}
		name = &validated
	}
	if err := validateQaNote(note); err != nil {
		return nil, err
	}
	return s.qa.UpdateRun(ctx, id, name, note)
}

func (s *qaService) DeleteRun(ctx context.Context, id string) error {
	return s.qa.DeleteRun(ctx, id)
}

func (s *qaService) SetResult(ctx context.Context, runID, testKey string, status, note *string) (*ent.QaTestResult, error) {
	if len(testKey) > 120 || !testKeyPattern.MatchString(testKey) {
		return nil, ErrInvalidTestKey
	}
	if err := validateQaNote(note); err != nil {
		return nil, err
	}
	var st *qatestresult.Status
	if status != nil {
		switch qatestresult.Status(*status) {
		case qatestresult.StatusSuccess, qatestresult.StatusError:
			v := qatestresult.Status(*status)
			st = &v
		default:
			return nil, ErrInvalidQaStatus
		}
	}
	// Unknown run must surface as 404, not as the upsert's FK conflict.
	if _, err := s.qa.GetRun(ctx, runID); err != nil {
		return nil, err
	}
	return s.qa.UpsertResult(ctx, runID, testKey, st, note)
}
