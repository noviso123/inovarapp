package domain

// CustomerPortalData is the authenticated, RLS-scoped data shown in the
// customer portal. Raw database rows remain JSON so legacy optional fields are
// preserved until their individual UI flows are migrated.
type CustomerPortalData struct {
	Customer   map[string]any   `json:"customer"`
	Appliances []map[string]any `json:"appliances"`
	Services   []map[string]any `json:"services"`
	Budgets    []map[string]any `json:"budgets"`
}
