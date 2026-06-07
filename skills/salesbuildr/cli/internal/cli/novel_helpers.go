// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"salesbuildr-pp-cli/internal/store"
)

// novelStoreLoadLimit is deliberately huge: the hand-built analytics commands
// aggregate over the whole synced corpus, and store.List silently caps at 200
// rows when limit <= 0.
const novelStoreLoadLimit = 1000000

// sbTimeLayouts covers the ISO-8601 shapes Salesbuildr returns (with and
// without fractional seconds, with Z or numeric offset, date-only).
var sbTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// parseSBTime parses a Salesbuildr timestamp string, returning the zero time
// on empty/unparseable input rather than erroring — missing dates are common
// and should degrade gracefully, not abort an aggregation.
func parseSBTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range sbTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// openNovelStore opens the local store for a hand-built transcendence command
// and emits the generated sync hints for resourceType before returning the
// handle. Callers own Close(). Pass resourceType "" for commands that scan
// multiple resources.
func openNovelStore(cmd *cobra.Command, flags *rootFlags, dbPath, resourceType string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("salesbuildr-cli")
	}
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, err
	}
	if !hintIfUnsynced(cmd, db, resourceType) {
		hintIfStale(cmd, db, resourceType, flags.maxAge)
	}
	return db, nil
}

// ---------------------------------------------------------------------------
// Quotes
// ---------------------------------------------------------------------------

// novelQuoteItem is one line item on a quote, parsed from the items[] array
// the sync command persists inside each quote's JSON document.
type novelQuoteItem struct {
	ID        string  `json:"id,omitempty"`
	Name      string  `json:"name,omitempty"`
	MPN       string  `json:"mpn,omitempty"`
	ProductID string  `json:"productId,omitempty"`
	Price     float64 `json:"price"`
	Cost      float64 `json:"cost"`
	Markup    float64 `json:"markup"`
	Quantity  float64 `json:"quantity"`
}

// effectiveQuantity treats a zero quantity as one unit so single-item lines
// without an explicit quantity still contribute their price to value sums.
func (i novelQuoteItem) effectiveQuantity() float64 {
	if i.Quantity == 0 {
		return 1
	}
	return i.Quantity
}

// effectiveMarkup prefers the explicit markup field and falls back to the
// computed price-over-cost percentage when markup is absent but both price
// and cost are known.
func (i novelQuoteItem) effectiveMarkup() (float64, bool) {
	if i.Markup != 0 {
		return i.Markup, true
	}
	if i.Cost > 0 && i.Price > 0 {
		return (i.Price - i.Cost) / i.Cost * 100, true
	}
	return 0, false
}

// candidateKeys returns every identity key this line item could match a
// catalog product under, strongest first (id, mpn, name).
func (i novelQuoteItem) candidateKeys() []string {
	keys := make([]string, 0, 3)
	if i.ProductID != "" {
		keys = append(keys, "id:"+i.ProductID)
	}
	if i.MPN != "" {
		keys = append(keys, "mpn:"+strings.ToLower(i.MPN))
	}
	if i.Name != "" {
		keys = append(keys, "name:"+strings.ToLower(i.Name))
	}
	return keys
}

// productKey identifies the catalog product a line item refers to: product id
// when linked, else MPN, else the lowercased item name.
func (i novelQuoteItem) productKey() string {
	if i.ProductID != "" {
		return "id:" + i.ProductID
	}
	if i.MPN != "" {
		return "mpn:" + strings.ToLower(i.MPN)
	}
	if i.Name != "" {
		return "name:" + strings.ToLower(i.Name)
	}
	return ""
}

// novelQuote is the parsed, store-backed view of a Salesbuildr quote used by
// the hand-built transcendence commands (stale, thin, funnel, whitespace,
// product velocity).
type novelQuote struct {
	ID         string
	Number     string
	Title      string
	Company    string
	CompanyID  string
	Status     string
	CreatedAt  time.Time
	SentAt     time.Time
	ApprovedAt time.Time
	DeclinedAt time.Time
	ExpiredAt  time.Time
	DeletedAt  time.Time
	Items      []novelQuoteItem
}

// value is the sum of line-item price x quantity — the dollar size of the quote.
func (q novelQuote) value() float64 {
	var total float64
	for _, i := range q.Items {
		total += i.Price * i.effectiveQuantity()
	}
	return total
}

// quoteLifecycleStages is the ordered funnel display list. lifecycleStage MUST
// return only values from this slice — the funnel render loop iterates it, so
// a label that is produced but not listed here silently disappears.
var quoteLifecycleStages = []string{"draft", "sent", "approved", "declined", "expired"}

// lifecycleStage classifies a quote into its furthest lifecycle stage using
// the lifecycle timestamps first and the status string as fallback.
func (q novelQuote) lifecycleStage() string {
	status := strings.ToLower(q.Status)
	switch {
	case !q.DeclinedAt.IsZero() || status == "declined":
		return "declined"
	case !q.ExpiredAt.IsZero() || status == "expired":
		return "expired"
	case !q.ApprovedAt.IsZero() || status == "approved":
		return "approved"
	case !q.SentAt.IsZero() || status == "sent":
		return "sent"
	default:
		return "draft"
	}
}

// isOpen reports whether the quote is still in play: not deleted and not in a
// terminal lifecycle stage.
func (q novelQuote) isOpen() bool {
	if !q.DeletedAt.IsZero() {
		return false
	}
	switch q.lifecycleStage() {
	case "declined", "expired":
		return false
	}
	status := strings.ToLower(q.Status)
	return status != "lost"
}

// ageReference returns the timestamp aging is measured from (approvedAt, else
// sentAt, else createdAt) plus the field label for display.
func (q novelQuote) ageReference() (time.Time, string) {
	if !q.ApprovedAt.IsZero() {
		return q.ApprovedAt, "approvedAt"
	}
	if !q.SentAt.IsZero() {
		return q.SentAt, "sentAt"
	}
	return q.CreatedAt, "createdAt"
}

// parseNovelQuote converts a decoded quote JSON document into a novelQuote.
// The company field is untyped upstream (string or {id,name} object); both
// shapes are accepted.
func parseNovelQuote(raw json.RawMessage) novelQuote {
	m := decodeObj(raw)
	q := novelQuote{
		ID:         gstr(m, "id"),
		Number:     gstr(m, "number"),
		Title:      gstr(m, "title"),
		Company:    firstNonEmpty(gobjName(m, "company"), gstr(m, "company")),
		CompanyID:  gobjID(m, "company"),
		Status:     statusLabel(m["status"], gstr(m, "statusId")),
		CreatedAt:  parseSBTime(gstr(m, "createdAt")),
		SentAt:     parseSBTime(gstr(m, "sentAt")),
		ApprovedAt: parseSBTime(gstr(m, "approvedAt")),
		DeclinedAt: parseSBTime(gstr(m, "declinedAt")),
		ExpiredAt:  parseSBTime(gstr(m, "expiredAt")),
		DeletedAt:  parseSBTime(gstr(m, "deletedAt")),
	}
	for _, itemRaw := range gArray(m, "items") {
		im := decodeObj(itemRaw)
		q.Items = append(q.Items, novelQuoteItem{
			ID:        gstr(im, "id"),
			Name:      gstr(im, "name"),
			MPN:       gstr(im, "mpn"),
			ProductID: firstNonEmpty(gobjID(im, "product"), gstr(im, "product")),
			Price:     gnum(im, "price"),
			Cost:      gnum(im, "cost"),
			Markup:    gnum(im, "markup"),
			Quantity:  gnum(im, "quantity"),
		})
	}
	return q
}

// loadQuotesHinted opens the store with sync hints for "quote", loads every
// synced quote, and closes the handle.
func loadQuotesHinted(cmd *cobra.Command, flags *rootFlags, dbPath string) ([]novelQuote, error) {
	db, err := openNovelStore(cmd, flags, dbPath, "quote")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	raws, err := db.List("quote", novelStoreLoadLimit)
	if err != nil {
		return nil, err
	}
	quotes := make([]novelQuote, 0, len(raws))
	for _, raw := range raws {
		quotes = append(quotes, parseNovelQuote(raw))
	}
	return quotes, nil
}

// ---------------------------------------------------------------------------
// Products
// ---------------------------------------------------------------------------

// novelProduct is the parsed catalog product view used by pricing drift,
// product velocity, and company whitespace.
type novelProduct struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	MPN    string  `json:"mpn,omitempty"`
	Vendor string  `json:"vendor,omitempty"`
	Price  float64 `json:"price"`
	Cost   float64 `json:"cost"`
	Markup float64 `json:"markup"`
	MSRP   float64 `json:"msrp"`
}

// productKey mirrors novelQuoteItem.productKey so quoted line items and
// catalog products resolve to the same identity space.
func (p novelProduct) productKeys() []string {
	keys := []string{"id:" + p.ID}
	if p.MPN != "" {
		keys = append(keys, "mpn:"+strings.ToLower(p.MPN))
	}
	if p.Name != "" {
		keys = append(keys, "name:"+strings.ToLower(p.Name))
	}
	return keys
}

func parseNovelProduct(raw json.RawMessage) novelProduct {
	m := decodeObj(raw)
	return novelProduct{
		ID:     gstr(m, "id"),
		Name:   gstr(m, "name"),
		MPN:    gstr(m, "mpn"),
		Vendor: firstNonEmpty(gobjName(m, "vendor"), gstr(m, "vendor")),
		Price:  gnum(m, "price"),
		Cost:   gnum(m, "cost"),
		Markup: gnum(m, "markup"),
		MSRP:   gnum(m, "msrp"),
	}
}

// loadProductsHinted opens the store with sync hints for "product", loads the
// whole catalog, and closes the handle.
func loadProductsHinted(cmd *cobra.Command, flags *rootFlags, dbPath string) ([]novelProduct, error) {
	db, err := openNovelStore(cmd, flags, dbPath, "product")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return loadProductsFromStore(db)
}

func loadProductsFromStore(db *store.Store) ([]novelProduct, error) {
	raws, err := db.List("product", novelStoreLoadLimit)
	if err != nil {
		return nil, err
	}
	products := make([]novelProduct, 0, len(raws))
	for _, raw := range raws {
		products = append(products, parseNovelProduct(raw))
	}
	return products, nil
}

// ---------------------------------------------------------------------------
// Opportunities
// ---------------------------------------------------------------------------

// novelOpportunity is the parsed pipeline view used by velocity, winrate, and
// mrr-forecast.
type novelOpportunity struct {
	ID             string
	Name           string
	Company        string
	Owner          string
	Stage          string
	Category       string
	Status         string
	Probability    float64
	MonthlyRevenue float64
	MonthlyProfit  float64
	OnetimeRevenue float64
	OneTimeProfit  float64
	MonthlyChurn   float64
	SalesCycleDays float64
	ClosedAt       time.Time
	CreatedAt      time.Time
	StageUpdatedAt time.Time
}

// isWon / isLost classify a closed opportunity from its status label. The API
// encodes status as a free-form statusId string; "won"/"win" and
// "lost"/"lose" substrings are the stable signal.
func (o novelOpportunity) isWon() bool {
	s := strings.ToLower(o.Status)
	return strings.Contains(s, "won") || strings.Contains(s, "win")
}

func (o novelOpportunity) isLost() bool {
	s := strings.ToLower(o.Status)
	return strings.Contains(s, "lost") || strings.Contains(s, "lose")
}

// isClosed reports whether the opportunity has left the open pipeline.
func (o novelOpportunity) isClosed() bool {
	return !o.ClosedAt.IsZero() || o.isWon() || o.isLost()
}

func parseNovelOpportunity(raw json.RawMessage) novelOpportunity {
	m := decodeObj(raw)
	return novelOpportunity{
		ID:             gstr(m, "id"),
		Name:           gstr(m, "name"),
		Company:        firstNonEmpty(gobjName(m, "company"), gstr(m, "company")),
		Owner:          firstNonEmpty(gobjName(m, "owner"), gstr(m, "owner"), gstr(m, "ownerId")),
		Stage:          firstNonEmpty(gobjName(m, "pipelineStage"), gstr(m, "pipelineStageDisplayValue"), gstr(m, "pipelineStage"), gstr(m, "pipelineStageId")),
		Category:       firstNonEmpty(gobjName(m, "category"), gstr(m, "categoryDisplayValue"), gstr(m, "category"), gstr(m, "categoryId")),
		Status:         statusLabel(m["statusId"], gstr(m, "statusId")),
		Probability:    gnum(m, "probability"),
		MonthlyRevenue: gnum(m, "monthlyRevenue"),
		MonthlyProfit:  gnum(m, "monthlyProfit"),
		OnetimeRevenue: gnum(m, "onetimeRevenue"),
		OneTimeProfit:  gnum(m, "oneTimeProfit"),
		MonthlyChurn:   gnum(m, "monthlyChurn"),
		SalesCycleDays: gnum(m, "salesCycleDurationDays"),
		ClosedAt:       parseSBTime(gstr(m, "closedAt")),
		CreatedAt:      parseSBTime(gstr(m, "createdAt")),
		StageUpdatedAt: parseSBTime(gstr(m, "stageUpdatedAt")),
	}
}

// loadOpportunitiesHinted opens the store with sync hints for "opportunity",
// loads the pipeline, and closes the handle.
func loadOpportunitiesHinted(cmd *cobra.Command, flags *rootFlags, dbPath string) ([]novelOpportunity, error) {
	db, err := openNovelStore(cmd, flags, dbPath, "opportunity")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	raws, err := db.List("opportunity", novelStoreLoadLimit)
	if err != nil {
		return nil, err
	}
	opps := make([]novelOpportunity, 0, len(raws))
	for _, raw := range raws {
		opps = append(opps, parseNovelOpportunity(raw))
	}
	return opps, nil
}

// ---------------------------------------------------------------------------
// Pricing books
// ---------------------------------------------------------------------------

// novelPricingBookProduct is one per-company price override inside a pricing book.
type novelPricingBookProduct struct {
	ProductID string  `json:"productId"`
	Price     float64 `json:"price"`
	Cost      float64 `json:"cost"`
}

type novelPricingBook struct {
	ID       string
	Name     string
	Products []novelPricingBookProduct
}

func parseNovelPricingBook(raw json.RawMessage) novelPricingBook {
	m := decodeObj(raw)
	b := novelPricingBook{
		ID:   gstr(m, "id"),
		Name: gstr(m, "name"),
	}
	for _, pRaw := range gArray(m, "products") {
		pm := decodeObj(pRaw)
		b.Products = append(b.Products, novelPricingBookProduct{
			ProductID: firstNonEmpty(gstr(pm, "productId"), gobjID(pm, "product")),
			Price:     gnum(pm, "price"),
			Cost:      gnum(pm, "cost"),
		})
	}
	return b
}

// loadPricingBooksHinted opens the store with sync hints for "pricing-book",
// loads every book, and closes the handle.
func loadPricingBooksHinted(cmd *cobra.Command, flags *rootFlags, dbPath string) ([]novelPricingBook, error) {
	db, err := openNovelStore(cmd, flags, dbPath, "pricing-book")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	raws, err := db.List("pricing-book", novelStoreLoadLimit)
	if err != nil {
		return nil, err
	}
	books := make([]novelPricingBook, 0, len(raws))
	for _, raw := range raws {
		books = append(books, parseNovelPricingBook(raw))
	}
	return books, nil
}

// ---------------------------------------------------------------------------
// Companies + external-identifier records (reconcile-psa, whitespace)
// ---------------------------------------------------------------------------

type novelCompany struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	ExternalIdentifier string `json:"externalIdentifier,omitempty"`
}

func loadCompaniesFromStore(db *store.Store) ([]novelCompany, error) {
	raws, err := db.List("company", novelStoreLoadLimit)
	if err != nil {
		return nil, err
	}
	companies := make([]novelCompany, 0, len(raws))
	for _, raw := range raws {
		m := decodeObj(raw)
		companies = append(companies, novelCompany{
			ID:                 gstr(m, "id"),
			Name:               gstr(m, "name"),
			ExternalIdentifier: gstr(m, "externalIdentifier"),
		})
	}
	return companies, nil
}

// extIDRecord is the minimal row reconcile-psa inspects on every PSA-synced
// resource: identity plus the external identifier that links it to the PSA.
type extIDRecord struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	ExternalIdentifier string `json:"externalIdentifier,omitempty"`
}

// loadExtIDRecords reads id/name/externalIdentifier for one resource type from
// the generic resources table. Contact rows compose name from first/last name.
func loadExtIDRecords(db *store.Store, resourceType string) ([]extIDRecord, error) {
	raws, err := db.List(resourceType, novelStoreLoadLimit)
	if err != nil {
		return nil, err
	}
	records := make([]extIDRecord, 0, len(raws))
	for _, raw := range raws {
		m := decodeObj(raw)
		name := gstr(m, "name")
		if name == "" {
			name = strings.TrimSpace(gstr(m, "firstName") + " " + gstr(m, "lastName"))
		}
		records = append(records, extIDRecord{
			ID:                 gstr(m, "id"),
			Name:               name,
			ExternalIdentifier: strings.TrimSpace(gstr(m, "externalIdentifier")),
		})
	}
	return records, nil
}
