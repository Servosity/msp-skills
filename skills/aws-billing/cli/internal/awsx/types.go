// Package awsx is the hand-built AWS SDK for Go v2 client layer for the
// aws-billing CLI. It performs real external AWS API calls (Cost Explorer,
// Organizations, EC2, CloudWatch, S3) through the native credential chain and
// SigV4 — the printed CLI never relies on the generated HTTP client for AWS.
//
// pp:client-call
package awsx

// Account mirrors the store "accounts" table columns.
type Account struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Email     string `json:"email"`
}

// CostLine mirrors the store "costs" table columns. Canonical breakdown lines
// carry account_id+service with empty usage_type; usage-type detail lines
// carry service+usage_type with empty account_id, so aggregate queries can
// avoid double-counting by filtering on usage_type.
type CostLine struct {
	ID          string  `json:"id"`
	PeriodStart string  `json:"period_start"`
	PeriodEnd   string  `json:"period_end"`
	Granularity string  `json:"granularity"`
	AccountID   string  `json:"account_id"`
	AccountName string  `json:"account_name"`
	Service     string  `json:"service"`
	Region      string  `json:"region"`
	UsageType   string  `json:"usage_type"`
	AmountUSD   float64 `json:"amount_usd"`
	Unit        string  `json:"unit"`
}

// InventoryResource mirrors the store "inventory" table columns. Used by the
// waste hunters; monthly_waste_usd > 0 marks a waste candidate.
type InventoryResource struct {
	ID              string  `json:"id"`
	ResourceType    string  `json:"resource_type"`
	ResourceID      string  `json:"resource_id"`
	AccountID       string  `json:"account_id"`
	Region          string  `json:"region"`
	State           string  `json:"state"`
	MonthlyCostUSD  float64 `json:"monthly_cost_usd"`
	MonthlyWasteUSD float64 `json:"monthly_waste_usd"`
	WasteReason     string  `json:"waste_reason"`
	Attrs           string  `json:"attrs"`
}

// Forecast mirrors the store "forecasts" table columns.
type Forecast struct {
	ID          string  `json:"id"`
	PeriodStart string  `json:"period_start"`
	PeriodEnd   string  `json:"period_end"`
	AccountID   string  `json:"account_id"`
	MeanUSD     float64 `json:"mean_usd"`
	LowerUSD    float64 `json:"lower_usd"`
	UpperUSD    float64 `json:"upper_usd"`
}

// DimensionValue is one value returned by Cost Explorer GetDimensionValues.
type DimensionValue struct {
	Value     string `json:"value"`
	Attr      string `json:"attribute,omitempty"`
	Dimension string `json:"dimension"`
}
