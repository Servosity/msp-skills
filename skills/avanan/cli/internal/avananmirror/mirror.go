// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// Package avananmirror ingests Avanan's query-shaped resources into the local
// SQLite store.
//
// The generated syncer can only mirror list-shaped GET endpoints, which for
// this API is just scopes and the MSP objects. The two highest-value
// resources — security events and SaaS entities — are POST-with-body queries
// paged by an opaque scrollId, so they need a purpose-built ingester. Without
// it the offline commands (triage, campaign, timeline, exceptions audit) have
// nothing to read.
//
// Two Avanan-specific behaviors drive the design:
//
//   - scrollId is a within-run cursor only. The vendor does not guarantee it
//     survives across runs, so incremental sync is expressed as a persisted
//     high-water timestamp per resource and scope, with scrollId used purely
//     to page a single window.
//   - Responses are enveloped as {responseEnvelope:{...}, responseData:[...]}.
//     A 200 with a non-zero envelope responseCode is still a failure.
package avananmirror

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"avanan-pp-cli/internal/cliutil"
	"avanan-pp-cli/internal/store"
)

// ResourceEvents and friends are the resource_type values written to the
// local store. They are stable identifiers: the offline commands query by
// them, so renaming one silently empties a command.
// These are deliberately prefixed. The generated syncer already writes the
// raw API shape for `exceptions` (and could later claim `events`/`entities`)
// into the same generic `resources` table. Sharing a resource_type would let
// two different row shapes occupy one namespace: `StoredException` unmarshals
// a raw API row without erroring, yielding entries with an empty match string,
// so `exceptions find` would scan hundreds of rows and confidently answer
// "not excepted anywhere". A separate namespace makes the two mirrors
// independent instead of silently poisoning each other.
const (
	ResourceEvents     = "avanan_events"
	ResourceEntities   = "avanan_entities"
	ResourceExceptions = "avanan_exceptions"
)

// MaxPageSize is the per-request record cap. Avanan does not document an upper
// bound; 100 matches the page size the generated syncer uses for the list
// endpoints and keeps a single page well inside the response size limit.
const MaxPageSize = 100

// Poster is the subset of the generated client this package needs. Narrowing
// it keeps the ingester unit-testable without constructing a full client.
type Poster interface {
	Post(ctx context.Context, path string, body any) (json.RawMessage, int, error)
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// Envelope is the response wrapper every SmartAPI endpoint returns.
type Envelope struct {
	ResponseEnvelope struct {
		RequestID      string `json:"requestId"`
		ResponseCode   int    `json:"responseCode"`
		ResponseText   string `json:"responseText"`
		AdditionalText string `json:"additionalText"`
		RecordsNumber  int    `json:"recordsNumber"`
		ScrollID       string `json:"scrollId"`
	} `json:"responseEnvelope"`
	ResponseData json.RawMessage `json:"responseData"`
}

// Decode unwraps an API response, surfacing an envelope-level failure as an
// error even when the HTTP status was 200.
func Decode(raw json.RawMessage) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parsing response envelope: %w", err)
	}
	// Avanan uses 0 and 200 interchangeably for success depending on endpoint.
	if code := env.ResponseEnvelope.ResponseCode; code != 0 && code != 200 {
		msg := strings.TrimSpace(env.ResponseEnvelope.ResponseText + " " + env.ResponseEnvelope.AdditionalText)
		if msg == "" {
			msg = "no detail supplied"
		}
		return nil, fmt.Errorf("API reported failure (responseCode %d): %s", code, msg)
	}
	return &env, nil
}

// Records splits an envelope's responseData into individual JSON objects.
// responseData is an array for query endpoints and a bare object for
// single-item fetches; both shapes are accepted.
func (e *Envelope) Records() ([]json.RawMessage, error) {
	if len(e.ResponseData) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(e.ResponseData))
	if trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal(e.ResponseData, &items); err != nil {
			return nil, fmt.Errorf("parsing responseData array: %w", err)
		}
		return items, nil
	}
	return []json.RawMessage{e.ResponseData}, nil
}

// Options controls an ingestion run.
type Options struct {
	// Since bounds the query window. Zero means "everything the API will give
	// us", which for a busy tenant is a lot — callers should always set it.
	Since time.Time
	// Scopes narrows the query to specific {farm}:{tenant} strings. Empty
	// means every scope the credential can reach.
	Scopes []string
	// MaxPages caps scrollId paging. Zero means unlimited; a positive value
	// bounds a first run against a large tenant.
	MaxPages int
	// PageSize overrides MaxPageSize.
	PageSize int
	// SaaS narrows the entity search to specific platforms. Empty means every
	// platform in EntitySaaS. The entity endpoint rejects a query with no
	// saas value, so there is no "all platforms in one call" option.
	SaaS []string
}

func (o Options) pageSize() int {
	if o.PageSize > 0 && o.PageSize <= MaxPageSize {
		return o.PageSize
	}
	return MaxPageSize
}

// Result reports what an ingestion run did.
type Result struct {
	Resource string `json:"resource"`
	Fetched  int    `json:"fetched"`
	Stored   int    `json:"stored"`
	Pages    int    `json:"pages"`
	Skipped  int    `json:"skipped_no_id"`
	Capped   bool   `json:"page_cap_hit"`
}

// idFields are checked in order when deriving a stable primary key. Avanan
// names the identifier differently per resource, and a record without any of
// them cannot be upserted without generating duplicates on every run.
//
// Dotted entries address a nested field. The entity search is the reason: its
// records carry no top-level identifier at all, nesting it under entityInfo,
// so a flat lookup skipped every single row. Verified live on 2026-08-22
// against smart-api-production-1-us, where a mirror fetched 343 entities and
// stored none of them while reporting success.
var idFields = []string{
	"eventId",
	"entityId",
	"entityInfo.entityId",
	"id",
	"exceptionId",
	"_id",
}

// lookupPath resolves a dotted path against a decoded JSON object.
func lookupPath(obj map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = obj
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func recordID(raw json.RawMessage) string {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	for _, f := range idFields {
		v, ok := lookupPath(obj, f)
		if !ok {
			continue
		}
		switch s := v.(type) {
		case string:
			if s != "" {
				return s
			}
		case float64:
			return fmt.Sprintf("%.0f", s)
		}
	}
	return ""
}

// EntitySaaS lists the platform identifiers the entity search accepts. The
// endpoint requires exactly one per query, so a full entity mirror is one pass
// per platform rather than a single call.
var EntitySaaS = []string{
	"office365_emails",
	"google_mail",
	"office365_onedrive",
	"office365_sharepoint",
	"google_drive",
	"ms_teams",
	"slack",
	"box2",
	"dropbox2",
}

// DiscoverScopes asks the API which {farm}:{tenant} scopes the credential can
// reach.
//
// A multi-scope app client rejects an unscoped request on the exception and
// sectool paths with HTTP 400 "Must provide a single scope in as \"scope\" in
// query string due to multi scope app client". Options.Scopes documents empty
// as "every scope the credential can reach", so the caller resolves it to the
// real list rather than sending one unscoped call that a multi-scope tenant
// cannot answer.
func DiscoverScopes(ctx context.Context, c Poster) ([]string, error) {
	raw, err := c.Get(ctx, "/v1.0/scopes", nil)
	if err != nil {
		return nil, fmt.Errorf("discovering scopes: %w", err)
	}
	env, err := Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("discovering scopes: %w", err)
	}
	if len(env.ResponseData) == 0 {
		return nil, nil
	}
	var scopes []string
	if err := json.Unmarshal(env.ResponseData, &scopes); err != nil {
		return nil, fmt.Errorf("parsing scopes: %w", err)
	}
	out := scopes[:0]
	for _, sc := range scopes {
		if sc = strings.TrimSpace(sc); sc != "" {
			out = append(out, sc)
		}
	}
	return out, nil
}

// Events ingests security events for the requested window.
func Events(ctx context.Context, c Poster, db *store.Store, opts Options) (Result, error) {
	requestData := map[string]any{}
	if !opts.Since.IsZero() {
		requestData["startDate"] = opts.Since.UTC().Format(time.RFC3339)
	}
	if len(opts.Scopes) > 0 {
		requestData["scopes"] = opts.Scopes
	}
	return ingestQuery(ctx, c, db, "/v1.0/event/query", ResourceEvents, requestData, opts)
}

// Entities ingests SaaS entities (emails, files, messages) for the window.
//
// Unlike the event query, /v1.0/search/query requires requestData.entityFilter
// carrying a saas value, and reads the window from inside that filter rather
// than from requestData. Sending the flat {startDate, pageSize} shape the event
// query accepts returns HTTP 422 "entityFilter Field required" and mirrors
// nothing, which is what this endpoint did before.
func Entities(ctx context.Context, c Poster, db *store.Store, opts Options) (Result, error) {
	platforms := opts.SaaS
	if len(platforms) == 0 {
		platforms = EntitySaaS
	}

	total := Result{Resource: ResourceEntities}
	var firstErr error
	succeeded := 0

	for _, saas := range platforms {
		if err := ctx.Err(); err != nil {
			return total, err
		}

		entityFilter := map[string]any{"saas": saas}
		if !opts.Since.IsZero() {
			entityFilter["startDate"] = opts.Since.UTC().Format(time.RFC3339)
		}
		requestData := map[string]any{"entityFilter": entityFilter}
		if len(opts.Scopes) > 0 {
			requestData["scopes"] = opts.Scopes
		}

		res, err := ingestQuery(ctx, c, db, "/v1.0/search/query", ResourceEntities, requestData, opts)
		total.Fetched += res.Fetched
		total.Stored += res.Stored
		total.Skipped += res.Skipped
		total.Pages += res.Pages
		total.Capped = total.Capped || res.Capped
		if err != nil {
			// A tenant that does not license a platform answers with an error
			// for that platform only. Keep going so one unlicensed SaaS does
			// not empty the whole entity mirror.
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", saas, err)
			}
			continue
		}
		succeeded++
	}

	if succeeded == 0 && firstErr != nil {
		return total, firstErr
	}
	return total, nil
}

// ingestQuery walks one query endpoint's scrollId pages into the store.
func ingestQuery(ctx context.Context, c Poster, db *store.Store, path, resource string, requestData map[string]any, opts Options) (Result, error) {
	res := Result{Resource: resource}

	scrollID := ""
	for {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if opts.MaxPages > 0 && res.Pages >= opts.MaxPages {
			res.Capped = true
			break
		}

		body := map[string]any{"requestData": cloneWithScroll(requestData, scrollID, opts.pageSize())}
		raw, _, err := c.Post(ctx, path, body)
		if err != nil {
			return res, fmt.Errorf("querying %s: %w", resource, err)
		}
		env, err := Decode(raw)
		if err != nil {
			return res, fmt.Errorf("%s: %w", resource, err)
		}
		records, err := env.Records()
		if err != nil {
			return res, fmt.Errorf("%s: %w", resource, err)
		}

		res.Pages++
		res.Fetched += len(records)

		for _, rec := range records {
			id := recordID(rec)
			if id == "" {
				// Storing an ID-less record would insert a duplicate on every
				// run. Count it so the caller can report the gap rather than
				// silently under-reporting the mirror size.
				res.Skipped++
				continue
			}
			if err := db.Upsert(resource, id, rec); err != nil {
				return res, fmt.Errorf("storing %s %s: %w", resource, id, err)
			}
			res.Stored++
		}

		next := env.ResponseEnvelope.ScrollID
		if next == "" || len(records) == 0 || next == scrollID {
			// An unchanged scrollId means the server is looping; stop rather
			// than page forever against a rate-limited API.
			break
		}
		scrollID = next
	}

	return res, nil
}

func cloneWithScroll(base map[string]any, scrollID string, pageSize int) map[string]any {
	out := make(map[string]any, len(base)+2)
	for k, v := range base {
		out[k] = v
	}
	out["pageSize"] = pageSize
	if scrollID != "" {
		out["scrollId"] = scrollID
	}
	return out
}

// ExceptionSource describes one of the seven exception sub-systems. They do
// not share a path shape, an ID scheme, or a response layout, which is exactly
// why a unified local table is worth building.
type ExceptionSource struct {
	// SubSystem is the stable discriminator written to the store and shown to
	// users. It is the vocabulary `exceptions find` and `exceptions audit`
	// report in, so it must stay human-meaningful.
	SubSystem string
	// Path is the listing endpoint.
	Path string
	// Engine is the vendor's internal identifier, retained for round-tripping
	// back to the write endpoints.
	Engine string
}

// ExceptionSources enumerates every exception surface the API exposes.
//
// The anti-phishing and spam families live under /v1.0/exceptions with a list
// side in the path; the malware, URL, and DLP families live under
// /v1.0/sectool-exceptions keyed by engine; anomaly and click-time protection
// live under a third prefix entirely.
func ExceptionSources() []ExceptionSource {
	return []ExceptionSource{
		{SubSystem: "anti-phishing-whitelist", Path: "/v1.0/exceptions/whitelist", Engine: "whitelist"},
		{SubSystem: "anti-phishing-blacklist", Path: "/v1.0/exceptions/blacklist", Engine: "blacklist"},
		{SubSystem: "spam-whitelist", Path: "/v1.0/exceptions/spam_whitelist", Engine: "spam_whitelist"},
		{SubSystem: "anti-malware", Path: "/v1.0/sectool-exceptions/checkpoint2/exceptions/hash", Engine: "checkpoint2"},
		{SubSystem: "url-reputation-allow", Path: "/v1.0/sectool-exceptions/avanan_url/exceptions/allow-url", Engine: "avanan_url"},
		{SubSystem: "url-reputation-block", Path: "/v1.0/sectool-exceptions/avanan_url/exceptions/block-url", Engine: "avanan_url"},
		{SubSystem: "dlp", Path: "/v1.0/sectool-exceptions/avanan_dlp/exceptions/hash", Engine: "avanan_dlp"},
		{SubSystem: "anomaly", Path: "/v1.0/sectools/anomaly/exceptions", Engine: "anomaly"},
		{SubSystem: "click-time-protection", Path: "/v1.0/sectools/click_time_protection/exceptions/items", Engine: "click_time_protection"},
	}
}

// StoredException is the unified row shape written for every sub-system.
// Flattening nine differently-shaped payloads into one schema is what makes a
// single cross-sub-system query possible.
type StoredException struct {
	SubSystem   string          `json:"sub_system"`
	Engine      string          `json:"engine"`
	ID          string          `json:"id"`
	MatchString string          `json:"match_string"`
	ListSide    string          `json:"list_side"`
	CreatedBy   string          `json:"created_by"`
	Comment     string          `json:"comment"`
	Raw         json.RawMessage `json:"raw"`
}

// matchFields are the keys, in priority order, that different sub-systems use
// to hold the value an exception matches on.
var matchFields = []string{
	"senderEmail", "senderDomain", "recipientEmail", "recipient",
	"url", "domain", "hash", "md5", "sha256",
	"value", "matchString", "name", "text", "listItemName", "item",
}

var creatorFields = []string{"createdBy", "added_by", "addedBy", "creator", "user"}
var commentFields = []string{"comment", "description", "reason", "note"}

// FlattenException normalizes one raw exception record into the unified shape.
func FlattenException(src ExceptionSource, raw json.RawMessage) StoredException {
	out := StoredException{SubSystem: src.SubSystem, Engine: src.Engine, Raw: raw}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return out
	}
	out.ID = recordID(raw)
	out.MatchString = firstString(obj, matchFields)
	out.CreatedBy = firstString(obj, creatorFields)
	out.Comment = firstString(obj, commentFields)

	switch {
	case strings.Contains(src.SubSystem, "whitelist"), strings.Contains(src.SubSystem, "allow"):
		out.ListSide = "allow"
	case strings.Contains(src.SubSystem, "blacklist"), strings.Contains(src.SubSystem, "block"):
		out.ListSide = "block"
	default:
		out.ListSide = "n/a"
	}
	return out
}

func firstString(obj map[string]any, keys []string) string {
	for _, k := range keys {
		if v, ok := obj[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// IsRateLimited reports whether err (or anything it wraps) is the typed
// rate-limit error the generated client raises once 429 retries are exhausted.
func IsRateLimited(err error) bool {
	var rl *cliutil.RateLimitError
	return errors.As(err, &rl)
}

// Exceptions ingests every exception sub-system into one local table.
//
// A sub-system that fails on its own merits is recorded and skipped rather
// than aborting the run: a tenant without a DLP licence returns 403 for that
// engine, and losing the other eight families to it would make the command
// useless for exactly the customers who need it most.
//
// Throttling is the one exception to that tolerance. A 429 does not mean "this
// engine has no entries", it means "we were not allowed to ask" — and a
// half-populated exception table makes `exceptions find` answer "not excepted
// anywhere" from data it never received. For a tool whose output gates
// allow/block decisions, that false negative is worse than no answer, so a
// rate-limit error aborts the whole ingest and is returned to the caller.
func Exceptions(ctx context.Context, c Poster, db *store.Store, opts Options) (Result, []error) {
	res := Result{Resource: ResourceExceptions}
	var problems []error

	// One request per scope. Sending a single scope param when several were
	// requested would mirror one tenant's exceptions and label them for none,
	// which makes `exceptions find` answer "not excepted anywhere" for every
	// other tenant from data it never received.
	scopeParams := []map[string]string{{}}
	if len(opts.Scopes) > 0 {
		scopeParams = scopeParams[:0]
		for _, sc := range opts.Scopes {
			scopeParams = append(scopeParams, map[string]string{"scope": sc})
		}
	}

	for _, src := range ExceptionSources() {
		for _, params := range scopeParams {
			if err := ctx.Err(); err != nil {
				problems = append(problems, err)
				return res, problems
			}

			raw, err := c.Get(ctx, src.Path, params)
			if err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", src.SubSystem, err))
				if IsRateLimited(err) {
					return res, problems
				}
				continue
			}
			env, err := Decode(raw)
			if err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", src.SubSystem, err))
				continue
			}
			records, err := env.Records()
			if err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", src.SubSystem, err))
				continue
			}

			res.Pages++
			res.Fetched += len(records)

			for _, rec := range records {
				flat := FlattenException(src, rec)
				if flat.ID == "" {
					// Fall back to a content-derived key so an ID-less entry is
					// still queryable and still deduplicates across runs.
					if flat.MatchString == "" {
						res.Skipped++
						continue
					}
					flat.ID = src.SubSystem + ":" + flat.MatchString
				}
				encoded, err := json.Marshal(flat)
				if err != nil {
					problems = append(problems, fmt.Errorf("%s: encoding %s: %w", src.SubSystem, flat.ID, err))
					continue
				}
				if err := db.Upsert(ResourceExceptions, src.SubSystem+"/"+scopeKey(params)+flat.ID, encoded); err != nil {
					problems = append(problems, fmt.Errorf("%s: storing %s: %w", src.SubSystem, flat.ID, err))
					continue
				}
				res.Stored++
			}
		}
	}

	return res, problems
}

// scopeKey namespaces stored rows by scope so two tenants' identically-numbered
// exceptions do not overwrite each other in the local table.
func scopeKey(params map[string]string) string {
	if sc := params["scope"]; sc != "" {
		return sc + "/"
	}
	return ""
}
