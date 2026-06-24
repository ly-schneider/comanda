package auth

import (
	"context"
)

type contextKey string

const (
	UserIDKey      contextKey = "userID"
	UserEmailKey   contextKey = "userEmail"
	UserNameKey    contextKey = "userName"
	IsAnonymousKey contextKey = "isAnonymous"
	AuthTypeKey    contextKey = "authType"
	DeviceIDKey    contextKey = "deviceID"
	DeviceTypeKey  contextKey = "deviceType"
	TenantIDKey    contextKey = "tenantID"
	SessionIDKey   contextKey = "sessionID"
	MemberRoleKey  contextKey = "memberRole"

	// IsPlatformAdminKey carries whether the authenticated user is a platform
	// admin (user.is_platform_admin). Set by the session middleware.
	IsPlatformAdminKey contextKey = "isPlatformAdmin"
	// PlatformContextKey marks a request on a platform host that resolved to no
	// tenant. Set by the tenant resolver.
	PlatformContextKey contextKey = "platformContext"
	// ActiveOrgIDKey carries the session's active_organization_id, read by the
	// user→tenant resolver to scope a platform-host request to the vendor's tenant.
	ActiveOrgIDKey contextKey = "activeOrgID"
)

type AuthType string

const (
	AuthTypeUser   AuthType = "user"
	AuthTypeDevice AuthType = "device"
)

type DeviceType string

const (
	DeviceTypePOS     DeviceType = "pos"
	DeviceTypeStation DeviceType = "station"
)

// GetTenantID returns the resolved tenant ID from ctx. The tenant-tx
// middleware sets this before binding the request to a tenant-scoped pgx
// transaction.
func GetTenantID(ctx context.Context) (string, bool) {
	tenantID, ok := ctx.Value(TenantIDKey).(string)
	return tenantID, ok
}

// RequireTenantID panics if the tenant ID is not in context. Only safe after
// the tenant resolver middleware has run.
func RequireTenantID(ctx context.Context) string {
	tenantID, ok := GetTenantID(ctx)
	if !ok {
		panic("auth: tenant ID not found in context - ensure tenant middleware is applied")
	}
	return tenantID
}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// WithIsPlatformAdmin records whether the authenticated user is a platform
// admin. Only ever set true after a DB-backed session lookup confirms
// user.is_platform_admin.
func WithIsPlatformAdmin(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, IsPlatformAdminKey, v)
}

// IsPlatformAdmin reports whether the request carries a platform-admin user.
func IsPlatformAdmin(ctx context.Context) bool {
	v, ok := ctx.Value(IsPlatformAdminKey).(bool)
	return ok && v
}

func WithActiveOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, ActiveOrgIDKey, orgID)
}

// WithSessionID records the id of the session row backing the request. The
// owner-invite accept flow uses it to set active_organization_id on the exact
// session.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, SessionIDKey, sessionID)
}

// GetSessionID reports false when no session id is in context.
func GetSessionID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(SessionIDKey).(string)
	return id, ok && id != ""
}

// WithMemberRole records the tenant-scoped org member.role resolved by
// RequireTenantRole, for telemetry.
func WithMemberRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, MemberRoleKey, role)
}

// GetMemberRole reports false when no member role is in context.
func GetMemberRole(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(MemberRoleKey).(string)
	return role, ok && role != ""
}

// GetActiveOrgID reports false when the session has no active organization
// (key unset or empty).
func GetActiveOrgID(ctx context.Context) (string, bool) {
	orgID, ok := ctx.Value(ActiveOrgIDKey).(string)
	return orgID, ok && orgID != ""
}

// WithPlatformContext marks the request as running on a platform host that
// resolved to no tenant.
func WithPlatformContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, PlatformContextKey, true)
}

// IsPlatformContext reports whether the request resolved to a no-tenant
// platform context.
func IsPlatformContext(ctx context.Context) bool {
	v, ok := ctx.Value(PlatformContextKey).(bool)
	return ok && v
}

func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok
}

// RequireUserID panics if the user ID is not in context.
// Only safe after auth middleware has validated the request.
func RequireUserID(ctx context.Context) string {
	userID, ok := GetUserID(ctx)
	if !ok {
		panic("auth: user ID not found in context - ensure auth middleware is applied")
	}
	return userID
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

func GetUserEmail(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(UserEmailKey).(string)
	return email, ok
}

func WithUserEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, UserEmailKey, email)
}

func GetUserName(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(UserNameKey).(string)
	return name, ok
}

func WithUserName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, UserNameKey, name)
}

func IsAnonymousUser(ctx context.Context) bool {
	isAnon, ok := ctx.Value(IsAnonymousKey).(bool)
	return ok && isAnon
}

func WithIsAnonymous(ctx context.Context, isAnonymous bool) context.Context {
	return context.WithValue(ctx, IsAnonymousKey, isAnonymous)
}

func GetAuthType(ctx context.Context) (AuthType, bool) {
	authType, ok := ctx.Value(AuthTypeKey).(AuthType)
	return authType, ok
}

func WithAuthType(ctx context.Context, authType AuthType) context.Context {
	return context.WithValue(ctx, AuthTypeKey, authType)
}

func GetDeviceID(ctx context.Context) (string, bool) {
	deviceID, ok := ctx.Value(DeviceIDKey).(string)
	return deviceID, ok
}

// RequireDeviceID panics if the device ID is not in context.
// Only safe after device auth middleware has validated the request.
func RequireDeviceID(ctx context.Context) string {
	deviceID, ok := GetDeviceID(ctx)
	if !ok {
		panic("auth: device ID not found in context - ensure device auth middleware is applied")
	}
	return deviceID
}

func WithDeviceID(ctx context.Context, deviceID string) context.Context {
	return context.WithValue(ctx, DeviceIDKey, deviceID)
}

func GetDeviceType(ctx context.Context) (DeviceType, bool) {
	deviceType, ok := ctx.Value(DeviceTypeKey).(DeviceType)
	return deviceType, ok
}

// RequireDeviceType panics if the device type is not in context.
// Only safe after device auth middleware has validated the request.
func RequireDeviceType(ctx context.Context) DeviceType {
	deviceType, ok := GetDeviceType(ctx)
	if !ok {
		panic("auth: device type not found in context - ensure device auth middleware is applied")
	}
	return deviceType
}

func WithDeviceType(ctx context.Context, deviceType DeviceType) context.Context {
	return context.WithValue(ctx, DeviceTypeKey, deviceType)
}

func IsDeviceAuth(ctx context.Context) bool {
	authType, ok := GetAuthType(ctx)
	return ok && authType == AuthTypeDevice
}

func IsUserAuth(ctx context.Context) bool {
	authType, ok := GetAuthType(ctx)
	return ok && authType == AuthTypeUser
}
