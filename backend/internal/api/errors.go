package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/repository"
	"backend/internal/response"
	"backend/internal/service"
)

// writeError writes a generated.Error JSON response with the given HTTP status,
// machine-readable code, and human-readable message.
func writeError(w http.ResponseWriter, status int, code, message string) {
	response.WriteJSON(w, status, generated.Error{
		Code:    code,
		Message: message,
	})
}

// writeOutOfStockError returns a 409 carrying the sold-out product/variant ids
// in details so the cart UI can flag the exact lines. The message stays generic;
// the web maps the code + item data to localized copy.
func writeOutOfStockError(w http.ResponseWriter, err *service.InsufficientInventoryError) {
	items := make([]map[string]any, 0, len(err.Items))
	for _, it := range err.Items {
		m := map[string]any{
			"productId": it.ProductID,
			"name":      it.Name,
			"available": it.Available,
			"requested": it.Requested,
		}
		if it.VariantID != "" {
			m["variantId"] = it.VariantID
		}
		items = append(items, m)
	}
	details := map[string]any{"items": items}
	response.WriteJSON(w, http.StatusConflict, generated.Error{
		Code:    "out_of_stock",
		Message: "Not enough stock for one or more items.",
		Details: &details,
	})
}

func writeProductUnavailableError(w http.ResponseWriter, err *service.ProductUnavailableError) {
	items := make([]map[string]any, 0, len(err.Items))
	for _, it := range err.Items {
		items = append(items, map[string]any{"productId": it.ProductID, "name": it.Name})
	}
	details := map[string]any{"items": items}
	response.WriteJSON(w, http.StatusConflict, generated.Error{
		Code:    "product_unavailable",
		Message: "One or more items are no longer available.",
		Details: &details,
	})
}

// writeEntError inspects err and maps it to an appropriate HTTP error response.
//
// Mapping:
//   - ent.IsNotFound / repository.ErrNotFound   -> 404 Not Found
//   - ent.IsConstraintError / repository.ErrConflict -> 409 Conflict
//   - ent.IsValidationError                      -> 400 Bad Request
//   - everything else                            -> 500 Internal Server Error
//
// The client only ever sees the generic codes/messages below — the underlying
// error is never serialized outward. For the 500 case the real cause is logged
// server-side (see logUnhandledError), so DB driver errors, permission/RLS
// denials, etc. stop vanishing into an opaque "An unexpected error occurred."
func writeEntError(w http.ResponseWriter, err error) {
	switch {
	case ent.IsNotFound(err) || errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "The requested resource was not found.")

	case ent.IsConstraintError(err) || errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "The resource already exists or violates a constraint.")

	case ent.IsValidationError(err):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())

	default:
		logUnhandledError(err)
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}

// writeRefundError maps refund-service errors to HTTP responses. Not-found and
// constraint errors get their precise status; genuine infra/Postgres failures
// are logged and returned as 500; the remaining service validation errors
// (over-refund, missing reason, not-refundable, unknown line) surface as 400
// with their message so the POS/admin UI can show why.
func writeRefundError(w http.ResponseWriter, err error) {
	switch {
	case ent.IsNotFound(err) || errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "The requested resource was not found.")

	case ent.IsConstraintError(err) || errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "The resource already exists or violates a constraint.")

	case errors.Is(err, service.ErrProviderRefund):
		logUnhandledError(err)
		writeError(w, http.StatusBadGateway, "provider_error", "The payment provider could not process the refund.")

	default:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			logUnhandledError(err)
			writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
			return
		}
		writeError(w, http.StatusBadRequest, "refund_failed", err.Error())
	}
}

// logUnhandledError records the real cause of a 500 server-side at Error level
// (zap's production config attaches a goroutine stacktrace automatically). The
// detail must never leak to the client, but without it a failing DB write is an
// untraceable generic 500. Postgres errors are unwrapped so the SQLSTATE, table,
// and constraint are searchable in the logs.
func logUnhandledError(err error) {
	if err == nil {
		return
	}

	fields := []zap.Field{zap.Error(err)}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		fields = append(fields,
			zap.String("pg_sqlstate", pgErr.Code),
			zap.String("pg_message", pgErr.Message),
		)
		if pgErr.Detail != "" {
			fields = append(fields, zap.String("pg_detail", pgErr.Detail))
		}
		if pgErr.TableName != "" {
			fields = append(fields, zap.String("pg_table", pgErr.TableName))
		}
		if pgErr.ConstraintName != "" {
			fields = append(fields, zap.String("pg_constraint", pgErr.ConstraintName))
		}
		// 42501 (insufficient_privilege) is either a missing GRANT for comanda_app
		// or an RLS WITH CHECK violation. Both look identical to the client (500)
		// and are the most common silent-500 cause, so call them out explicitly.
		if pgErr.Code == "42501" {
			fields = append(fields, zap.String("diagnosis",
				"permission denied or RLS violation: check comanda_app grants on this table and the app.tenant_id / app.is_platform_admin GUCs"))
		}
	}

	zap.L().Error("unhandled server error", fields...)
}
