package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/qatestresult"
	"backend/internal/repository"
	"backend/internal/response"
	"backend/internal/service"
)

func (h *Handlers) ListQaRuns(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	runs, counts, err := h.qa.ListRuns(r.Context())
	if err != nil {
		writeEntError(w, err)
		return
	}
	items := make([]generated.QaRun, 0, len(runs))
	for _, run := range runs {
		items = append(items, toAPIQaRun(run, counts[run.ID]))
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) CreateQaRun(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	var body generated.QaRunCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	created, err := h.qa.CreateRun(r.Context(), body.Name, body.Note)
	if err != nil {
		if !writeQaValidationError(w, err) {
			writeEntError(w, err)
		}
		return
	}
	response.WriteJSON(w, http.StatusCreated, toAPIQaRun(created, repository.QaRunCounts{}))
}

func (h *Handlers) GetQaRun(w http.ResponseWriter, r *http.Request, runID string) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	run, results, err := h.qa.GetRun(r.Context(), runID)
	if err != nil {
		writeEntError(w, err)
		return
	}
	items := make([]generated.QaResult, 0, len(results))
	counts := repository.QaRunCounts{}
	for _, res := range results {
		items = append(items, toAPIQaResult(res))
		if res.Status != nil {
			switch *res.Status {
			case qatestresult.StatusSuccess:
				counts.Success++
			case qatestresult.StatusError:
				counts.Error++
			}
		}
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{
		"run":     toAPIQaRun(run, counts),
		"results": items,
	})
}

func (h *Handlers) UpdateQaRun(w http.ResponseWriter, r *http.Request, runID string) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	var body generated.QaRunUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	updated, err := h.qa.UpdateRun(r.Context(), runID, body.Name, body.Note)
	if err != nil {
		if !writeQaValidationError(w, err) {
			writeEntError(w, err)
		}
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIQaRun(updated, repository.QaRunCounts{}))
}

func (h *Handlers) DeleteQaRun(w http.ResponseWriter, r *http.Request, runID string) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	if err := h.qa.DeleteRun(r.Context(), runID); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) UpsertQaResult(w http.ResponseWriter, r *http.Request, runID string, testKey string) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	var body generated.QaResultUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	var status *string
	if body.Status != nil {
		s := string(*body.Status)
		status = &s
	}
	stored, err := h.qa.SetResult(r.Context(), runID, testKey, status, body.Note)
	if err != nil {
		if !writeQaValidationError(w, err) {
			writeEntError(w, err)
		}
		return
	}
	response.WriteJSON(w, http.StatusOK, toAPIQaResult(stored))
}

// writeQaValidationError reports whether it handled err as a 400.
func writeQaValidationError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, service.ErrInvalidRunName):
		writeError(w, http.StatusBadRequest, "invalid_run_name", "Run name must be 1-120 characters")
	case errors.Is(err, service.ErrInvalidQaNote):
		writeError(w, http.StatusBadRequest, "invalid_note", "Note is too long")
	case errors.Is(err, service.ErrInvalidTestKey):
		writeError(w, http.StatusBadRequest, "invalid_test_key", "Unknown test key format")
	case errors.Is(err, service.ErrInvalidQaStatus):
		writeError(w, http.StatusBadRequest, "invalid_qa_status", "status must be success or error")
	default:
		return false
	}
	return true
}

func toAPIQaRun(run *ent.QaTestRun, counts repository.QaRunCounts) generated.QaRun {
	return generated.QaRun{
		Id:           run.ID,
		Name:         run.Name,
		Note:         run.Note,
		SuccessCount: counts.Success,
		ErrorCount:   counts.Error,
		CreatedAt:    run.CreatedAt,
		UpdatedAt:    ptr(run.UpdatedAt),
	}
}

func toAPIQaResult(res *ent.QaTestResult) generated.QaResult {
	out := generated.QaResult{
		TestKey:   res.TestKey,
		Note:      res.Note,
		UpdatedAt: res.UpdatedAt,
	}
	if res.Status != nil {
		out.Status = ptr(generated.QaResultStatus(*res.Status))
	}
	return out
}
