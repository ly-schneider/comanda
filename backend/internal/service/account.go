package service

import (
	"context"
	"database/sql"
	"strings"

	"backend/internal/database"
)

// ManageableTenant is a tenant the signed-in user may manage from the account
// hub. Role is empty for a platform admin who isn't a member.
type ManageableTenant struct {
	ID          string
	Slug        string
	Name        string
	DisplayName string
	LogoURL     string
	Role        string
}

type AccountService interface {
	ListManageableTenants(ctx context.Context, userID string, isPlatformAdmin bool) ([]ManageableTenant, error)
}

// accountService runs on the privileged pool: the account hub lists tenants
// before any single tenant is bound, so the RLS-scoped app pool would return
// nothing for a non-platform-admin's cross-tenant membership lookup.
type accountService struct {
	db *sql.DB
}

func NewAccountService(privileged *database.PrivilegedClient) AccountService {
	return &accountService{db: privileged.DB}
}

func (s *accountService) ListManageableTenants(ctx context.Context, userID string, isPlatformAdmin bool) ([]ManageableTenant, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if isPlatformAdmin {
		rows, err = s.db.QueryContext(ctx, `
			SELECT t.id, t.slug, t.name, s.display_name, s.logo_url, m.role
			FROM tenant t
			LEFT JOIN settings s ON s.tenant_id = t.id
			LEFT JOIN member m ON m.organization_id = t.organization_id AND m.user_id = $1
			ORDER BY t.name`, userID)
	} else {
		// The hub is the settings tier only; app_admin and below stay on the tenant host.
		rows, err = s.db.QueryContext(ctx, `
			SELECT t.id, t.slug, t.name, s.display_name, s.logo_url, m.role
			FROM tenant t
			JOIN member m ON m.organization_id = t.organization_id AND m.user_id = $1 AND m.role IN ('owner', 'admin')
			LEFT JOIN settings s ON s.tenant_id = t.id
			ORDER BY t.name`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]ManageableTenant, 0)
	for rows.Next() {
		var (
			id, slug, name          string
			displayName, logo, role sql.NullString
		)
		if err := rows.Scan(&id, &slug, &name, &displayName, &logo, &role); err != nil {
			return nil, err
		}
		dn := name
		if v := strings.TrimSpace(displayName.String); displayName.Valid && v != "" {
			dn = v
		}
		out = append(out, ManageableTenant{
			ID:          id,
			Slug:        slug,
			Name:        name,
			DisplayName: dn,
			LogoURL:     logo.String,
			Role:        role.String,
		})
	}
	return out, rows.Err()
}
