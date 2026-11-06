// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet exec fans one read-only XAPI GET out to every PBX.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"threecx-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/client"
	"threecx-pp-cli/internal/fleet"
)

func newNovelFleetExecCmd(flags *rootFlags) *cobra.Command {
	var params, rawReqs []string
	var raw bool
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "exec <xapi-path>",
		Short: "Run any read-only Configuration API query on every client phone system and get one merged table with a pbx column",
		Long: `Run any read-only Configuration API query on every client phone system and get one merged table with a pbx column.

Use this command to run an arbitrary read-only XAPI query (users, DIDs, queues, reports) on every registered PBX and merge the results with a pbx column. Do NOT use this command for trunk status, version drift, renewals or backup state; use 'fleet trunks', 'fleet versions', 'fleet licences' or 'fleet backups' instead.

The path is relative to /xapi/v1 and is always sent as a GET. OData options go in
--param ('$filter=...', '$select=...', '$top=50'). Password, PIN, secret and key
fields are replaced with "[redacted]". A PBX that fails (unreachable, 401, 403
without the System Owner role, below the 8SC licence) becomes a row with an error.`,
		Example: strings.Trim(`
  threecx-cli fleet exec /Users --param '$select=Number,FirstName,LastName' --agent
  threecx-cli fleet exec /DidNumbers --only acme,globex --json
  threecx-cli fleet exec /Queues --param '$expand=Agents' --pbx acme --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "path=/SystemStatus",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet exec")
			}
			if raw {
				return runRawGets(cmd, flags, rawReqs)
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one XAPI path is required, e.g. /Users"))
			}
			path, err := cleanXAPIPath(args[0])
			if err != nil {
				return usageErr(err)
			}
			query, err := parseSyncKVFlags(params, "--param")
			if err != nil {
				return usageErr(err)
			}
			pbxes, _, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(pbxes) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			rows, failed := collect(cmd, &scope, flags, pbxes, []fleet.Request{{Path: path, Params: query}},
				func(p fleet.PBX, bodies []json.RawMessage) ([]map[string]any, error) {
					items := fleet.Items(bodies[0])
					for _, item := range items {
						item["pbx"] = p.Name
					}
					return items, nil
				},
				func(p fleet.PBX, err error) map[string]any {
					return map[string]any{"pbx": p.Name, "error": err.Error()}
				})
			if err := render(cmd, flags, rows, []string{"pbx", "error"}, "No rows.", func(r map[string]any) map[string]any { return r }); err != nil {
				return err
			}
			return allFailed(failed, len(pbxes))
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "Query parameter key=value, repeatable (e.g. '$filter=Enabled eq true')")
	cmd.Flags().BoolVar(&raw, "raw", false, "Internal: run the --req GETs against the pinned PBX and print their bodies")
	cmd.Flags().StringArrayVar(&rawReqs, "req", nil, "Internal: one path?query GET for --raw, repeatable")
	_ = cmd.Flags().MarkHidden("raw")
	_ = cmd.Flags().MarkHidden("req")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}

// runRawGets is the per-PBX child primitive behind every fleet command: the
// requested GETs with this process's pinned PBX credential, printed as one
// JSON array of redacted bodies.
func runRawGets(cmd *cobra.Command, flags *rootFlags, rawReqs []string) error {
	if pinnedPBX == "" {
		return usageErr(fmt.Errorf("--raw needs a selected PBX; use --pbx <name>"))
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
		if q.Path, err = cleanXAPIPath(q.Path); err != nil {
			return usageErr(err)
		}
		reqs = append(reqs, q)
	}
	flags.noCache = true
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	// XAPI GETs do not redirect. Refuse any redirect so a hop cannot leave
	// /xapi/v1 or land on a denied operation.
	if c.HTTPClient != nil {
		hc := *c.HTTPClient
		hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("refusing redirect to %s", req.URL.Redacted())
		}
		c.HTTPClient = &hc
	}
	bodies := make([]json.RawMessage, 0, len(reqs))
	for _, q := range reqs {
		ctx, cancel := boundCtx(cmd.Context(), flags)
		body, err := getAllPages(ctx, c, q.Path, q.Params)
		cancel()
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

// propertyRead reports a path that ends in a direct property read of a
// value that cannot be judged safe: Value, Contents, $value, or a property
// whose name looks like a credential (/Trunks(1)/AuthPassword).
func propertyRead(p string) bool {
	last := p[strings.LastIndex(p, "/")+1:]
	if i := strings.IndexByte(last, '('); i >= 0 {
		return false // an entity key or function call, not a property
	}
	switch strings.ToLower(last) {
	case "value", "contents", "$value":
		return true
	}
	return strings.Contains(p[1:], "/") && fleet.SecretField(last)
}

// maxPages bounds @odata.nextLink following for one collection.
const maxPages = 200

// getAllPages GETs path and follows OData @odata.nextLink pages on the same
// PBX, merging "value" arrays, so collections are never silently truncated.
func getAllPages(ctx context.Context, c *client.Client, path string, query map[string]string) (json.RawMessage, error) {
	body, err := c.Get(ctx, path, query)
	if err != nil {
		return nil, classifyAPIErrorOnly(err)
	}
	var page map[string]json.RawMessage
	if json.Unmarshal(body, &page) != nil || page["@odata.nextLink"] == nil {
		return body, nil
	}
	var all []json.RawMessage
	for n := 0; ; n++ {
		var items []json.RawMessage
		if raw, ok := page["value"]; ok {
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, apiErr(fmt.Errorf("%s: unexpected page shape: %w", path, err))
			}
		}
		all = append(all, items...)
		var next string
		if raw, ok := page["@odata.nextLink"]; !ok || json.Unmarshal(raw, &next) != nil || next == "" {
			break
		}
		if n+1 >= maxPages {
			return nil, apiErr(fmt.Errorf("%s: more than %d pages; narrow the query with $filter or $top", path, maxPages))
		}
		nextPath, nextQuery, err := sameHostNextLink(c, next)
		if err != nil {
			return nil, apiErr(fmt.Errorf("%s: %w", path, err))
		}
		if body, err = c.Get(ctx, nextPath, nextQuery); err != nil {
			return nil, classifyAPIErrorOnly(err)
		}
		page = nil
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, apiErr(fmt.Errorf("%s: unexpected page shape: %w", path, err))
		}
	}
	merged, err := json.Marshal(map[string]any{"value": all})
	return merged, err
}

// sameHostNextLink turns an absolute or relative @odata.nextLink into a path
// under /xapi/v1 plus query, refusing links that point at another host.
func sameHostNextLink(c *client.Client, next string) (string, map[string]string, error) {
	u, err := url.Parse(next)
	if err != nil {
		return "", nil, fmt.Errorf("bad @odata.nextLink: %w", err)
	}
	if u.IsAbs() {
		base, err := url.Parse(client.ResolvedBaseURL(c.BaseURL, c.Config.TemplateVars))
		if err != nil || !strings.EqualFold(u.Host, base.Host) || u.Scheme != base.Scheme {
			return "", nil, fmt.Errorf("@odata.nextLink points outside this PBX")
		}
	}
	if u.RawPath != "" && u.RawPath != u.Path {
		return "", nil, fmt.Errorf("@odata.nextLink has an encoded path")
	}
	p, err := cleanXAPIPath(strings.TrimPrefix(u.Path, "/xapi/v1"))
	if err != nil {
		return "", nil, fmt.Errorf("@odata.nextLink rejected: %w", err)
	}
	q := map[string]string{}
	for k, v := range u.Query() {
		if len(v) > 0 {
			q[k] = v[0]
		}
	}
	return p, q, nil
}

// cleanXAPIPath accepts "/Users", "Users", or "/xapi/v1/Users" and refuses
// anything that could leave the PBX's /xapi/v1 tree.
func cleanXAPIPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/xapi/v1")
	// A trailing slash must not hide a property or a function from the rules below.
	p = "/" + strings.Trim(p, "/")
	switch {
	case p == "/":
		return "", fmt.Errorf("an XAPI path is required, e.g. /Users")
	case strings.Contains(p, "//"), strings.Contains(p, ".."), strings.ContainsAny(p, "?#%\\ \t\r\n"):
		return "", fmt.Errorf("invalid XAPI path %q: give a plain path like /Users (no %%-encoding) and pass query options with --param", p)
	case len(p) > 512:
		return "", fmt.Errorf("XAPI path too long")
	case propertyRead(p):
		return "", fmt.Errorf("%s reads a single property that may be a credential; select the fields you need with --param '$select=...' on the entity instead", p)
	case deniedXAPIPath.MatchString(p), cliutil.MintsCredential(p):
		return "", fmt.Errorf("%s is not allowed in fleet exec: it returns credentials or changes state on every PBX; run it against one system with --pbx and the dedicated command", p)
	}
	return p, nil
}

// deniedXAPIPath blocks GET operations that mint or return credentials
// (GenerateApiKey, GenerateProvLink, APIToken, GetMicrosoftAccessToken,
// GetOAuth, MyTokens, SecurityTokens), grant vendor remote access
// (GetRequestHelpLink), allocate objects (Init*), or stream files
// (Download*, Export*). These are GETs in the spec but unsafe to fan out.
var deniedXAPIPath = regexp.MustCompile(`(?i)(^|/)(MyTokens|SecurityTokens)(\(|/|$)|Pbx\.(Generate\w*|Init\w*|APIToken|GetMicrosoftAccessToken|GetOAuth|GetRequestHelpLink|Download\w*|Export\w*)(\(|$)`)
