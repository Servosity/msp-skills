// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// Hand-authored. `policies decode`; shared policy fetch used by `policies clone`.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if policies, _, err := root.Find([]string{"policies"}); err == nil && policies != root {
			addNovelCommandIfAbsent(policies, newNovelPoliciesDecodeCmd(flags))
			addNovelCommandIfAbsent(policies, newNovelPoliciesCloneCmd(flags))
		}
	})
}

// policyInputMaxBytes bounds a saved policy read from --file or stdin. Real
// policy documents are far smaller; a larger input is refused rather than
// silently truncated into a confusing JSON parse error.
const policyInputMaxBytes = 64 << 20

// readPolicyInput reads at most policyInputMaxBytes from r.
func readPolicyInput(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, policyInputMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > policyInputMaxBytes {
		return nil, fmt.Errorf("input is larger than %d MiB", policyInputMaxBytes>>20)
	}
	return raw, nil
}

// readPolicyFile reads a saved policy JSON file the user named with --file.
func readPolicyFile(file string) ([]byte, error) {
	// Open the path as given: filepath.Clean would resolve "link/../x" lexically
	// and could open a different file than the shell would.
	f, err := os.Open(file) // #nosec G304 -- the user names the file to decode
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return readPolicyInput(f)
}

// fetchPolicyDoc returns the GET /v2/policies/{uuid} body, or a saved copy.
func fetchPolicyDoc(cmd *cobra.Command, flags *rootFlags, uuid, file string) (map[string]any, error) {
	var raw []byte
	var err error
	switch {
	case file == "-":
		raw, err = readPolicyInput(cmd.InOrStdin())
	case file != "":
		raw, err = readPolicyFile(file)
	default:
		c, cerr := flags.newClient()
		if cerr != nil {
			return nil, cerr
		}
		ctx, cancel := boundCtx(cmd.Context(), flags)
		defer cancel()
		raw, err = c.Get(ctx, replacePathParam(esetHost("policy-management", "/v2/policies/{policyUuid}"), "policyUuid", uuid), nil)
		if err != nil {
			return nil, classifyAPIError(cmd.OutOrStdout(), err, flags)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("reading policy: %w", err)
	}
	var doc map[string]any
	if err := esetDecodeExact(raw, &doc); err != nil {
		return nil, fmt.Errorf("policy is not a JSON object: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("policy is not a JSON object")
	}
	return doc, nil
}

func newNovelPoliciesDecodeCmd(flags *rootFlags) *cobra.Command {
	var find, path, file string
	var raw bool
	var maxMatches int
	cmd := &cobra.Command{
		Use:   "decode [policy-uuid]",
		Short: "Decode a policy's base64 and LZMA settings blobs into readable JSON",
		Long: "Fetch a policy and decode every PolicyData blob in its features: base64 layers,\n" +
			"the JSON header, and the ar archive whose *.lzma members (endpoint.lzma) hold the\n" +
			"product settings. Use --find to list only matching settings paths, --path to pull one\n" +
			"value out, or --file to decode a policy saved with 'policies get --json'.\n\n" +
			"ESET Connect has no call that edits an existing policy's settings; inspect here, then\n" +
			"use 'policies clone' or 'policies create' to roll out a changed copy.",
		Example: strings.Trim(`
  eset-protect-cli policies decode 0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f --find firewall
  eset-protect-cli policies decode 0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f --path '[0].archive_members[0].decoded.Settings'
  eset-protect-cli policies get 0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f --json > policy.json && eset-protect-cli policies decode --file policy.json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "policy-uuid=example-value",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "policies decode")
			}
			if len(args) == 0 && file == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass a policy UUID or --file <policy.json>"))
			}
			if find != "" && path != "" {
				return usageErr(fmt.Errorf("--find and --path are mutually exclusive"))
			}
			uuid := ""
			if len(args) > 0 {
				uuid = args[0]
			}
			doc, err := fetchPolicyDoc(cmd, flags, uuid, file)
			if err != nil {
				return err
			}
			found := make([]decodedPolicyData, 0)
			collectPolicyData(doc, "$", &found)

			// Re-read the decoded blobs as generic JSON so --find/--path walk them.
			var tree any
			buf, _ := json.Marshal(found)
			_ = esetDecodeExact(buf, &tree)

			var out any
			switch {
			case find != "":
				matches := make([]policyMatch, 0)
				findInJSON(tree, "$", strings.ToLower(find), maxMatches, &matches)
				view := map[string]any{"policy_uuid": uuid, "find": find, "matches": matches}
				// A blob or member that failed to decode was not searched; say so
				// rather than presenting a partial search as a clean no-match.
				if errs := policyDecodeErrors(found); len(errs) > 0 {
					view["incomplete"] = true
					view["decode_errors"] = errs
				}
				if len(matches) == 0 {
					view["note"] = fmt.Sprintf("no decoded setting key or value contains %q", find)
				}
				out = view
			case path != "":
				v, perr := valueAtPath(tree, path)
				if perr != nil {
					return usageErr(fmt.Errorf("--path %q: %w (run without --path to see the decoded tree)", path, perr))
				}
				out = map[string]any{"policy_uuid": uuid, "path": path, "value": v}
			default:
				policy := any(doc)
				if !raw {
					policy = omitPolicyDataBlobs(doc)
				}
				out = map[string]any{"policy": policy, "policy_data": found}
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&find, "find", "", "Case-insensitive text to find in decoded setting keys and values (e.g. firewall)")
	cmd.Flags().StringVar(&path, "path", "", "Dotted path into the decoded list to print one value, e.g. '[0].archive_members[0].decoded.Settings'")
	cmd.Flags().StringVar(&file, "file", "", "Decode a saved policy JSON file instead of fetching ('-' reads stdin)")
	cmd.Flags().BoolVar(&raw, "raw", false, "Keep the raw base64 blobs in the policy section of the output")
	cmd.Flags().IntVar(&maxMatches, "max-matches", 50, "Maximum --find matches to return")
	return cmd
}
