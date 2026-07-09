package enttenant

import (
	"sort"
	"testing"

	"backend/internal/generated/ent/admininvite"
	"backend/internal/generated/ent/bundleslot"
	"backend/internal/generated/ent/bundleslotoption"
	"backend/internal/generated/ent/category"
	"backend/internal/generated/ent/club100redemption"
	"backend/internal/generated/ent/device"
	"backend/internal/generated/ent/devicebinding"
	"backend/internal/generated/ent/deviceproduct"
	"backend/internal/generated/ent/idempotency"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/jeton"
	"backend/internal/generated/ent/migrate"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/orderlineredemption"
	"backend/internal/generated/ent/orderpayment"
	"backend/internal/generated/ent/product"
	"backend/internal/generated/ent/productvariant"
	"backend/internal/generated/ent/refund"
	"backend/internal/generated/ent/refundline"
	"backend/internal/generated/ent/settings"
	"backend/internal/generated/ent/stationenrolltoken"
	"backend/internal/generated/ent/tenantdomain"
	"backend/internal/generated/ent/tenantplugin"
	"backend/internal/generated/ent/volunteercampaign"
	"backend/internal/generated/ent/volunteercampaignproduct"
	"backend/internal/generated/ent/volunteerredemption"
)

const tenantColumnName = "tenant_id"

// typeToTable maps each Ent type name in scopedTypes to its storage table.
// Kept here (test-side) so the assertion below is fully self-checking.
var typeToTable = map[string]string{
	"Product":                  product.Table,
	"ProductVariant":           productvariant.Table,
	"Category":                 category.Table,
	"Jeton":                    jeton.Table,
	"BundleSlot":               bundleslot.Table,
	"BundleSlotOption":         bundleslotoption.Table,
	"Order":                    order.Table,
	"OrderLine":                orderline.Table,
	"OrderPayment":             orderpayment.Table,
	"OrderLineRedemption":      orderlineredemption.Table,
	"InventoryLedger":          inventoryledger.Table,
	"Club100Redemption":        club100redemption.Table,
	"Settings":                 settings.Table,
	"Idempotency":              idempotency.Table,
	"Device":                   device.Table,
	"DeviceBinding":            devicebinding.Table,
	"DeviceProduct":            deviceproduct.Table,
	"AdminInvite":              admininvite.Table,
	"VolunteerCampaign":        volunteercampaign.Table,
	"VolunteerCampaignProduct": volunteercampaignproduct.Table,
	"VolunteerRedemption":      volunteerredemption.Table,
	"TenantDomain":             tenantdomain.Table,
	"Refund":                   refund.Table,
	"RefundLine":               refundline.Table,
	"TenantPlugin":             tenantplugin.Table,
	"StationEnrollToken":       stationenrolltoken.Table,
}

// TestScopedTypesMatchSchema asserts the Go-layer guard set (scopedTypes)
// matches exactly the Ent schemas carrying a tenant_id column. A new
// tenant-scoped schema added without being listed in scopedTypes — or a type
// listed without the mixin — fails here, so the ORM-layer guard can never
// silently drift from the schema.
func TestScopedTypesMatchSchema(t *testing.T) {
	// Tables that actually carry tenant_id, straight from generated migrate data.
	tablesWithTenantID := map[string]bool{}
	for _, tbl := range migrate.Tables {
		for _, col := range tbl.Columns {
			if col.Name == tenantColumnName {
				tablesWithTenantID[tbl.Name] = true
				break
			}
		}
	}

	// Tables the guard claims to cover.
	guardedTables := map[string]bool{}
	for typeName := range scopedTypes {
		table, ok := typeToTable[typeName]
		if !ok {
			t.Fatalf("scopedTypes contains %q with no typeToTable mapping in this test", typeName)
		}
		guardedTables[table] = true
	}

	if missing := diff(tablesWithTenantID, guardedTables); len(missing) > 0 {
		t.Errorf("tables with tenant_id NOT guarded by enttenant.scopedTypes: %v", missing)
	}
	if extra := diff(guardedTables, tablesWithTenantID); len(extra) > 0 {
		t.Errorf("enttenant.scopedTypes guards tables that have no tenant_id column: %v", extra)
	}
}

func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
