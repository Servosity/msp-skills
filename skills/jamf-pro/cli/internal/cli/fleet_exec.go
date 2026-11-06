// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet exec fans one read-only GET out to every registered Jamf Pro instance.
// pp:data-source live

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/client"
	"jamf-pro-pp-cli/internal/fleet"
)

func newNovelFleetExecCmd(flags *rootFlags) *cobra.Command {
	var params, rawReqs []string
	var raw, freshToken bool
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "exec <api-path>",
		Short: "Run any read-only Jamf Pro or Classic API GET on every client instance and get one merged table with an instance column",
		Long: `Run any read-only Jamf Pro or Classic API GET on every client instance and get one merged table with an instance column.

Use this command for an arbitrary read-only query (any /api/... or /JSSResource/... GET) across every registered instance. Do NOT use this command to locate a device or for the built-in rollups; use 'fleet find', 'fleet posture', 'fleet patch' or 'fleet health' instead.

The path starts with /api/ (Jamf Pro API) or /JSSResource/ (Classic API) and is
always sent as a GET. Query parameters go in --param and may repeat
(--param section=GENERAL --param section=HARDWARE). Jamf Pro API collections are
read to the end: every page is fetched until totalCount is reached, unless you
pass --param page=N yourself. Password, secret, token, key and PIN fields are
replaced with "[redacted]"; whole configuration documents (profile payloads,
app preferences, script contents) become a "[redacted sha256:...]" digest; and
endpoints that return credentials (LAPS passwords and password audits,
FileVault and recovery-lock keys, lock PINs, OAuth) are refused.
An instance that fails (unreachable, 401, missing privilege) becomes a row with
an error.`,
		Example: strings.Trim(`
  jamf-pro-cli fleet exec /api/v1/jamf-pro-version --agent
  jamf-pro-cli fleet exec /api/v3/computer-groups/smart-groups --only acme,globex --json
  jamf-pro-cli fleet exec /JSSResource/policies --tag gold --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "path=/api/v1/jamf-pro-version",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet exec")
			}
			if raw {
				return runRawGets(cmd, flags, rawReqs, freshToken)
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one API path is required, e.g. /api/v1/jamf-pro-version"))
			}
			path, err := cleanJamfPath(args[0])
			if err != nil {
				return usageErr(err)
			}
			query, err := parseParamValues(params)
			if err != nil {
				return usageErr(err)
			}
			instances, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(instances) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			rows, failed := collect(cmd, &scope, flags, instances, []fleet.Request{{Path: path, Query: query}},
				func(p fleet.Instance, bodies []json.RawMessage) ([]map[string]any, error) {
					items, err := fleet.Items(bodies[0])
					if err != nil {
						return nil, err
					}
					for _, item := range items {
						item["instance"] = p.Name
					}
					return items, nil
				},
				func(p fleet.Instance, err error) map[string]any {
					return map[string]any{"instance": p.Name, "error": err.Error()}
				})
			if err := render(cmd, flags, rows, []string{"instance", "error"}, "No rows.", execColumns(rows), func(r map[string]any) map[string]any { return r }); err != nil {
				return err
			}
			return allFailed(failed, len(instances))
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "Query parameter key=value, repeatable (e.g. 'filter=general.name==\"MBP*\"', section=HARDWARE)")
	cmd.Flags().BoolVar(&raw, "raw", false, "Internal: run the --req GETs against the pinned instance and print their bodies")
	cmd.Flags().StringArrayVar(&rawReqs, "req", nil, "Internal: one path?query GET for --raw, repeatable")
	cmd.Flags().BoolVar(&freshToken, "fresh-token", false, "Internal: with --raw, mint a new token instead of reusing a cached one")
	_ = cmd.Flags().MarkHidden("fresh-token")
	_ = cmd.Flags().MarkHidden("raw")
	_ = cmd.Flags().MarkHidden("req")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}

// execColumns orders arbitrary result columns: instance first, error last,
// every other key sorted.
func execColumns(rows []map[string]any) []string {
	seen := map[string]bool{"instance": true, "error": true}
	var keys []string
	for _, r := range rows {
		for k := range r {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return append(append([]string{"instance"}, keys...), "error")
}

// parseParamValues reads repeatable key=value flags, keeping repeated keys.
func parseParamValues(params []string) (url.Values, error) {
	out := url.Values{}
	for _, kv := range params {
		k, v, ok := strings.Cut(kv, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("--param %q: use key=value", kv)
		}
		out.Add(k, v)
	}
	return out, nil
}

// runRawGets is the per-instance child primitive behind every fleet command:
// the requested GETs with this process's pinned credential, printed as one
// JSON array of redacted bodies.
func runRawGets(cmd *cobra.Command, flags *rootFlags, rawReqs []string, freshToken bool) error {
	if pinnedInstance == "" {
		return usageErr(fmt.Errorf("--raw needs a selected instance; use --instance <name>"))
	}
	if len(rawReqs) == 0 {
		return usageErr(fmt.Errorf("--raw needs at least one --req"))
	}
	reqs := make([]fleet.Request, 0, len(rawReqs))
	for _, arg := range rawReqs {
		q, err := fleet.DecodeRequest(arg)
		if err != nil {
			return usageErr(err)
		}
		if q.Path, err = cleanJamfPath(q.Path); err != nil {
			return usageErr(err)
		}
		reqs = append(reqs, q)
	}
	flags.noCache = true
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	if freshToken && c.Config != nil {
		// Drop any cached token so the first request mints with the current secret.
		c.Config.AccessToken = ""
		c.Config.AuthHeaderVal = ""
	}
	// These GETs do not redirect. Refuse any redirect so a hop cannot leave
	// the instance or land on a denied endpoint.
	if c.HTTPClient != nil {
		hc := *c.HTTPClient
		hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return errors.New("refusing a redirect; check the server URL")
		}
		c.HTTPClient = &hc
	}
	bodies := make([]json.RawMessage, 0, len(reqs))
	for _, q := range reqs {
		body, err := getAllJamfPages(cmd.Context(), flags, c, q.Path, q.Query)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			body = json.RawMessage("null")
		}
		redacted, err := fleet.RedactJSON(body)
		if err != nil {
			return apiErr(fmt.Errorf("non-JSON response from %s", q.Path))
		}
		bodies = append(bodies, redacted)
	}
	out, err := json.Marshal(bodies)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return err
}

// maxJamfPages bounds one collection walk. At the documented default page
// size of 100 that is 100,000 records per request.
const maxJamfPages = 1000

// getAllJamfPages GETs path and, when the answer is a Jamf Pro API page
// ({"totalCount":N,"results":[...]}) holding fewer results than totalCount,
// fetches the following pages until every record is read. Pages are 0-based
// (the vendor's page parameter default is 0). A caller-supplied page pins
// one page and disables the walk. If totalCount changes, or a page comes back
// empty before totalCount is reached, the collection changed while it was
// being read: that is an error, never a silently short or stale answer.
func getAllJamfPages(ctx context.Context, flags *rootFlags, c *client.Client, path string, query url.Values) (json.RawMessage, error) {
	get := func(q url.Values) (json.RawMessage, error) {
		cctx, cancel := boundCtx(ctx, flags)
		defer cancel()
		body, err := c.GetWithHeadersNoCacheValues(cctx, path, q, nil)
		if err != nil {
			return nil, classifyAPIErrorOnly(err)
		}
		return body, nil
	}
	body, err := get(query)
	if err != nil {
		return nil, err
	}
	if query.Has("page") {
		return body, nil
	}
	total, results, ok := jamfPage(body)
	if !ok {
		return body, nil
	}
	seen := map[string]bool{}
	if err := noRepeats(path, results, seen); err != nil {
		return nil, err
	}
	switch {
	case len(results) > total:
		return nil, apiErr(fmt.Errorf("%s: first page holds %d records but totalCount is %d", path, len(results), total))
	case len(results) == total:
		return body, nil
	case len(results) == 0:
		return nil, apiErr(fmt.Errorf("%s: first page is empty but totalCount is %d", path, total))
	}
	// The first page was full, so its length is the effective page size
	// (the caller's page-size, or the server default of 100).
	size := len(results)
	all := results
	for page := 1; len(all) < total; page++ {
		if page >= maxJamfPages {
			return nil, apiErr(fmt.Errorf("%s: more than %d pages; narrow the query with a filter", path, maxJamfPages))
		}
		q := url.Values{}
		for k, v := range query {
			q[k] = append([]string(nil), v...)
		}
		q.Set("page", strconv.Itoa(page))
		q.Set("page-size", strconv.Itoa(size))
		next, err := get(q)
		if err != nil {
			return nil, err
		}
		pageTotal, items, ok := jamfPage(next)
		switch {
		case !ok:
			return nil, apiErr(fmt.Errorf("%s: page %d has an unexpected shape", path, page))
		case pageTotal != total:
			return nil, apiErr(fmt.Errorf("%s: the collection changed while it was read (totalCount %d, then %d); run again", path, total, pageTotal))
		case len(items) == 0:
			return nil, apiErr(fmt.Errorf("%s: page %d was empty after %d of %d records; the collection changed while it was read, run again", path, page, len(all), total))
		}
		if err := noRepeats(path, items, seen); err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	if len(all) > total {
		return nil, apiErr(fmt.Errorf("%s: read %d records but totalCount is %d; the collection changed while it was read, run again", path, len(all), total))
	}
	return json.Marshal(map[string]any{"totalCount": total, "results": all})
}

// identityKeys are the record identifiers the Jamf Pro API collections use,
// in preference order (computers and most objects: id; mobile devices:
// mobileDeviceId; software update statuses: osUpdatesStatusId; MDM
// commands: uuid).
var identityKeys = []string{"id", "mobileDeviceId", "osUpdatesStatusId", "managementId", "udid", "uuid"}

// noRepeats fails when a record appears twice across pages: an unstable
// ordering shifted records between pages, so others were skipped. Records
// are identified by their documented id field, or by their full content
// when they carry none. The error names the record's position and a digest
// of its identity, never the identifier itself (it comes from the server).
func noRepeats(path string, items []json.RawMessage, seen map[string]bool) error {
	for i, it := range items {
		var rec map[string]json.RawMessage
		key := ""
		if json.Unmarshal(it, &rec) == nil {
			for _, k := range identityKeys {
				if v, ok := rec[k]; ok && len(v) > 0 && string(v) != "null" {
					key = k + "=" + string(v)
					break
				}
			}
		}
		if key == "" {
			sum := sha256.Sum256(it)
			key = "sha256=" + hex.EncodeToString(sum[:])
		}
		if seen[key] {
			sum := sha256.Sum256([]byte(key))
			return apiErr(fmt.Errorf("%s: the record at position %d (identity digest %s) was returned twice while paging; the order shifted and records may have been skipped, run again", path, i, hex.EncodeToString(sum[:6])))
		}
		seen[key] = true
	}
	return nil
}

// jamfPage decodes a Jamf Pro API collection page.
func jamfPage(body json.RawMessage) (int, []json.RawMessage, bool) {
	var page struct {
		TotalCount *int              `json:"totalCount"`
		Results    []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(body, &page); err != nil || page.TotalCount == nil || page.Results == nil {
		return 0, nil, false
	}
	return *page.TotalCount, page.Results, true
}

// cleanJamfPath accepts "/api/..." or "/JSSResource/..." and refuses anything
// that could leave the instance or read a credential. Percent-escapes are
// allowed (Classic API names with spaces need them); every check runs on the
// decoded path too, so an escape cannot smuggle a traversal or a denied name.
func cleanJamfPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	decoded, err := url.PathUnescape(p)
	if err != nil {
		return "", fmt.Errorf("invalid path %q: bad %%-escape", p)
	}
	switch {
	case !strings.HasPrefix(decoded, "/api/") && !strings.HasPrefix(decoded, "/JSSResource/"):
		return "", fmt.Errorf("path %q must start with /api/ (Jamf Pro API) or /JSSResource/ (Classic API)", p)
	case strings.Contains(p, "://"), strings.ContainsAny(p, "?#\\ \t\r\n"):
		return "", fmt.Errorf("invalid path %q: give a plain path like /api/v1/scripts and pass query options with --param", p)
	// Braces are refused: the client expands {server}-style templates in a
	// path after this check, which could turn an allowed path into a denied one.
	case strings.Contains(decoded, ".."), strings.Contains(decoded, "//"), strings.ContainsAny(decoded, "?#\\{}"), hasControl(decoded):
		return "", fmt.Errorf("invalid path %q", p)
	case len(p) > 1024:
		return "", fmt.Errorf("API path too long")
	case deniedJamfPath.MatchString(decoded), deniedJamfPath.MatchString(p):
		return "", fmt.Errorf("%s is not allowed in fleet exec: it returns credentials, tokens or files", p)
	}
	return p, nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// deniedJamfPath blocks GETs that return credentials or secrets (LAPS
// passwords and password audits, FileVault and recovery-lock keys, device lock PINs, OAuth and
// session tokens, minted API client secrets) or files instead of JSON.
var deniedJamfPath = regexp.MustCompile(`(?i)(filevault|recovery-?lock|lock-?pin|/password(/|$)|set-password|/view-|client-credentials|/oauth|/auth(/|$)|session-token|upload-token|/download(/|$)|/export|attachments|/fileuploads|local-admin-password/.*/audit(/|$))`)
