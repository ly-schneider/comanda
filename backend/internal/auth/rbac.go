package auth

// Permission represents a specific action that can be authorized. User roles no
// longer drive authorization: tenant access is membership-based (see
// tenant_rbac.go) and platform access is the user.is_platform_admin flag.
// Permissions remain only to model device capabilities (POS / Station terminals).
type Permission string

const (
	// POS permissions
	PermPOSAccess       Permission = "pos:access"
	PermPOSOrdersCreate Permission = "pos:orders:create"
	PermPOSPayments     Permission = "pos:payments"

	// Station permissions
	PermStationAccess Permission = "station:access"
	PermStationVerify Permission = "station:verify"
	PermStationRedeem Permission = "station:redeem"
)

// DevicePermissions maps device types to their permitted actions.
// Device permissions are separate from user roles.
var DevicePermissions = map[DeviceType]map[Permission]bool{
	DeviceTypePOS: {
		PermPOSAccess:       true,
		PermPOSOrdersCreate: true,
		PermPOSPayments:     true,
	},
	DeviceTypeStation: {
		PermStationAccess: true,
		PermStationVerify: true,
		PermStationRedeem: true,
	},
}

// HasDevicePermission checks if a device type has a specific permission.
func HasDevicePermission(deviceType DeviceType, perm Permission) bool {
	perms, ok := DevicePermissions[deviceType]
	if !ok {
		return false
	}
	return perms[perm]
}
