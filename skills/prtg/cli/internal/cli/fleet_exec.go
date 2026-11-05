// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet exec fans one read-only API v2 GET out to every core.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/client"
	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/fleet"
)

func newNovelFleetExecCmd(flags *rootFlags) *cobra.Command {
	var params, rawReqs []string
	var raw, all bool
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "exec <api-path>",
		Short: "Run any read-only API v2 query on every core and get one merged table with a core column",
		Long: `Run any read-only API v2 query on every core and get one merged table with a core column.

Use this command to run an arbitrary read-only API v2 GET (users, groups, probes, devices with a filter) on every registered core and merge the results with a core column. Do NOT use it for alarms, counts, hygiene or shift changes; use 'fleet alarms', 'fleet status', 'fleet drift' or 'fleet handoff' instead.

The path is relative to /api/v2 and is always sent as a GET. Query parameters go
in --param (filter, limit, offset, include). --all pages a list with limit/offset
until a short page. Password, key, token and similar fields are replaced with
"[redacted]". A core that fails (unreachable, 401, 403) becomes a row with an error.`,
		Example: strings.Trim(`
  prtg-cli fleet exec /experimental/probes --agent
  prtg-cli fleet exec /devices --param 'filter=status = down' --all --json
  prtg-cli fleet exec /version --cores acme,globex --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "path=/version",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet exec")
			}
			if raw {
				return runRawRequests(cmd, flags, rawReqs)
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one API v2 path is required, e.g. /experimental/probes"))
			}
			path, err := cleanAPIPath(args[0])
			if err != nil {
				return usageErr(err)
			}
			query := map[string]string{}
			for _, p := range params {
				k, v, ok := strings.Cut(p, "=")
				if !ok || strings.TrimSpace(k) == "" {
					return usageErr(fmt.Errorf("--param %q: use key=value", p))
				}
				query[strings.TrimSpace(k)] = v
			}
			method := fleet.MethodGet
			if all {
				method = fleet.MethodGetAll
				if _, ok := query["limit"]; !ok {
					query["limit"] = "3000"
				}
			}
			cores, done, err := scope.resolveOrEmpty(cmd, flags)
			if err != nil || done {
				return err
			}
			rows, failed := collect(cmd, &scope, flags, cores, []fleet.Request{{Method: method, Path: path, Params: query}},
				func(c fleet.Core, bodies []json.RawMessage) ([]map[string]any, error) {
					body, err := bodyOrErr(bodies[0])
					if err != nil {
						return nil, err
					}
					items := fleet.Items(body)
					for _, item := range items {
						item["core"] = c.Name
					}
					return items, nil
				},
				func(c fleet.Core, err error) map[string]any {
					return map[string]any{"core": c.Name, "error": err.Error()}
				})
			if err := render(cmd, flags, rows, nil, "No rows."); err != nil {
				return err
			}
			return allFailed(failed, len(cores))
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "Query parameter key=value, repeatable (e.g. 'filter=status = down', 'limit=100')")
	cmd.Flags().BoolVar(&all, "all", false, "Page a list endpoint with limit/offset until a short page")
	cmd.Flags().BoolVar(&raw, "raw", false, "Internal: run the --req requests against the pinned core and print their bodies")
	cmd.Flags().StringArrayVar(&rawReqs, "req", nil, "Internal: one [METHOD ]path?query request for --raw, repeatable")
	_ = cmd.Flags().MarkHidden("raw")
	_ = cmd.Flags().MarkHidden("req")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}

var apiPathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)

// cleanAPIPath accepts a path relative to /api/v2 (a leading /api/v2 is
// dropped). It must not carry a scheme, host, query, fragment or "..".
func cleanAPIPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if strings.HasPrefix(p, "/api/v2/") {
		p = strings.TrimPrefix(p, "/api/v2")
	}
	if strings.Contains(p, "..") || strings.Contains(p, "//") || !apiPathPattern.MatchString(p) {
		return "", fmt.Errorf("path %q must be an API v2 path like /experimental/probes (no scheme, host, query or ..)", p)
	}
	if cliutil.StateChangingGET(p) {
		return "", fmt.Errorf("path %q changes PRTG state even as a GET (it renews the session); fleet requests are reads only", p)
	}
	return p, nil
}

// writePaths are the only POSTs the raw child sends: PRTG's filter-based
// bulk actions used by fleet ack, pause and resume.
var writePaths = regexp.MustCompile(`^/(sensors/acknowledge|(sensors|devices|groups|probes)/(pause|resume|scan))$`)

// scanOnePath is the per-object scan now that follows a fleet resume.
var scanOnePath = regexp.MustCompile(`^/(sensors|devices|groups|probes)/[0-9]+/scan$`)

// runRawRequests is the per-core child primitive behind every fleet command:
// the requested calls with this process's pinned core key, printed as one
// JSON array of redacted bodies. A failed request becomes a pp_error body so
// one 403 does not hide the rest of the batch.
func runRawRequests(cmd *cobra.Command, flags *rootFlags, rawReqs []string) error {
	if pinnedCore == "" {
		return usageErr(fmt.Errorf("--raw needs a selected core; use --core <name>"))
	}
	if len(rawReqs) == 0 {
		return usageErr(fmt.Errorf("--raw needs at least one --req"))
	}
	reqs := make([]fleet.Request, 0, len(rawReqs))
	writes := false
	for _, arg := range rawReqs {
		q, err := fleet.DecodeRequest(arg)
		if err != nil {
			return usageErr(err)
		}
		if q.Path, err = cleanAPIPath(q.Path); err != nil {
			return usageErr(err)
		}
		if q.Method == fleet.MethodPost {
			// The marker and allowlist keep this from being a general write
			// path; anything it sends is also reachable through the generated
			// multi-* commands, so it adds no capability beyond them.
			if os.Getenv(fleetChildEnv) != "1" || !(writePaths.MatchString(q.Path) || scanOnePath.MatchString(q.Path)) {
				return usageErr(fmt.Errorf("--raw only sends bulk acknowledge/pause/resume/scan POSTs on behalf of a fleet command"))
			}
			writes = true
		}
		reqs = append(reqs, q)
	}
	var bodies []string
	if writes {
		data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
		if err != nil {
			return usageErr(fmt.Errorf("reading request bodies: %w", err))
		}
		if err := json.Unmarshal(data, &bodies); err != nil || len(bodies) != len(reqs) {
			return usageErr(fmt.Errorf("--raw POST expects a JSON array with one body per --req on stdin"))
		}
		// Same guard as the fleet action engine: a bulk action always names
		// a non-empty PRTG filter, never "everything"; a per-object scan
		// carries no body.
		for i, q := range reqs {
			if q.Method != fleet.MethodPost {
				continue
			}
			if scanOnePath.MatchString(q.Path) {
				if strings.TrimSpace(bodies[i]) != "" {
					return usageErr(fmt.Errorf("--raw POST %s takes no body", q.Path))
				}
				continue
			}
			var b struct {
				Filter string `json:"filter"`
			}
			if json.Unmarshal([]byte(bodies[i]), &b) != nil || strings.TrimSpace(b.Filter) == "" {
				return usageErr(fmt.Errorf("--raw POST %s needs a body with a non-empty filter", q.Path))
			}
		}
	}
	if writes && cliutil.IsAnyHarness() {
		return writeHarnessRefusal(cmd.OutOrStdout(), flags, "fleet write")
	}
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	// Fleet bodies can carry secrets (the license key) that are redacted
	// only on output; never let the response cache write them to disk.
	c.NoCache = true
	// The generated client bounds each HTTP call with --timeout; the parent
	// runner bounds the whole batch, so no batch-wide deadline here.
	ctx := cmd.Context()
	out := make([]json.RawMessage, 0, len(reqs))
	for i, q := range reqs {
		var body json.RawMessage
		switch q.Method {
		case fleet.MethodPost:
			var status int
			if strings.TrimSpace(bodies[i]) == "" {
				body, status, err = c.Post(ctx, q.Path, nil)
			} else {
				body, status, err = c.Post(ctx, q.Path, json.RawMessage(bodies[i]))
			}
			// A per-object scan answers 204 when it ran; 202 means the object
			// was created recently and the scan did not happen.
			if err == nil && scanOnePath.MatchString(q.Path) && status != 204 {
				out = append(out, fleet.ErrorBody(status, fmt.Sprintf("scan not performed: %s answered HTTP %d (204 expected; 202 = object not available yet)", q.Path, status)))
				continue
			}
		case fleet.MethodGetAll:
			body, err = getAllPages(ctx, c, q)
		default:
			body, err = c.GetNoCache(ctx, q.Path, q.Params)
		}
		if err != nil {
			out = append(out, rawError(err))
			continue
		}
		if len(strings.TrimSpace(string(body))) == 0 {
			body = json.RawMessage("null")
		}
		red, rerr := fleet.RedactJSON(body)
		if rerr != nil {
			out = append(out, fleet.ErrorBody(0, "non-JSON response from "+q.Path))
			continue
		}
		out = append(out, red)
	}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = cmd.OutOrStdout().Write(append(data, '\n'))
	return err
}

// getAllPages walks a list with limit/offset until a page shorter than the
// limit, returning one merged array. A list longer than fleet.MaxListPages
// pages is an error, never a silently shortened result (the live-dogfood
// harness alone curtails to one page).
func getAllPages(ctx context.Context, c *client.Client, q fleet.Request) (json.RawMessage, error) {
	limit, err := strconv.Atoi(q.Params["limit"])
	if err != nil || limit <= 0 || limit > 3000 {
		limit = 3000 // PRTG's documented maximum page size
	}
	offset, _ := strconv.Atoi(q.Params["offset"])
	if offset < 0 {
		offset = 0
	}
	dogfood := cliutil.IsDogfoodEnv()
	pages := fleet.MaxListPages
	if dogfood {
		pages = 1
	}
	merged := make([]json.RawMessage, 0, limit)
	for page := 0; page < pages; page++ {
		body, err := c.GetNoCache(ctx, q.Path, pageParams(q.Params, offset, limit))
		if err != nil {
			return nil, err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(body, &items); err != nil {
			if page == 0 {
				return body, nil // not a list: return it as is
			}
			return nil, fmt.Errorf("page %d of %s is not a list", page+1, q.Path)
		}
		merged = append(merged, items...)
		if len(items) < limit {
			return json.Marshal(merged)
		}
		offset += len(items)
	}
	if dogfood {
		return json.Marshal(merged)
	}
	// Every page was full: probe once more so a list that ends exactly on
	// the cap is still a complete answer.
	body, err := c.GetNoCache(ctx, q.Path, pageParams(q.Params, offset, 1))
	if err != nil {
		return nil, err
	}
	var more []json.RawMessage
	if json.Unmarshal(body, &more) == nil && len(more) == 0 {
		return json.Marshal(merged)
	}
	return nil, fmt.Errorf("%s has more than %d rows (%d pages of %d); raise limit (max 3000) or narrow it with a PRTG filter", q.Path, len(merged), pages, limit)
}

// rawError renders a client error as a per-request failure.
func rawError(err error) json.RawMessage {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return fleet.ErrorBody(apiErr.StatusCode, cliutil.SanitizeErrorBody(apiErr.Error()))
	}
	return fleet.ErrorBody(0, cliutil.SanitizeErrorBody(err.Error()))
}

// pageParams copies params with the given offset and limit.
func pageParams(params map[string]string, offset, limit int) map[string]string {
	out := maps.Clone(params)
	if out == nil {
		out = map[string]string{}
	}
	out["offset"] = strconv.Itoa(offset)
	out["limit"] = strconv.Itoa(limit)
	return out
}
