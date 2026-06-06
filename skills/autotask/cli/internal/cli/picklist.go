// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// entityNameRe bounds the picklist entity argument to a bare PascalCase
// entity name after normalization (letters and digits only).
var entityNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// newNovelPicklistCmd resolves the label<->ID value map inside one picklist
// field from /{Entity}/entityInformation/fields — the integer-ID decoder ring
// every Autotask filter and report needs. Live by default with write-through
// caching, local fallback when the API is unreachable.
// pp:data-source auto
func newNovelPicklistCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "picklist [entity] [field]",
		Short: "Print the label-to-ID map for any picklist field (status, priority, queue) of an entity.",
		Long: `Fetch an entity's field definitions and print the picklist values (label, value, active, default) for one field. Autotask stores most categorical fields as integer IDs; this is the decoder ring for building filters and reading reports.

Use this command to resolve the label<->ID values inside one picklist field. Do NOT use it to list all of an entity's fields; use the entity's 'query-field-definitions' command.`,
		Example: strings.Trim(`
  autotask-cli picklist Tickets status
  autotask-cli picklist Tickets priority --agent
  autotask-cli picklist time-entries timeEntryType --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "entity=Tickets;field=status",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 2 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("both entity and field are required, e.g. picklist Tickets status"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would fetch entityInformation fields for %s and resolve picklist %q\n", args[0], args[1])
				return nil
			}
			entityPascal, entityKebab := normalizeEntityName(args[0])
			// The entity becomes a URL path segment; reject anything beyond
			// a bare entity name so `/`, `..`, `?`, `#`, `%` can't rewrite
			// the request path or query (defense-in-depth; same host either way).
			if !entityNameRe.MatchString(entityPascal) {
				return usageErr(fmt.Errorf("invalid entity %q: must be a bare entity name like Tickets or time-entries", args[0]))
			}
			fieldName := strings.TrimSpace(args[1])

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			path := "/" + entityPascal + "/entityInformation/fields"
			data, prov, err := resolveReadWithStrategy(cmd.Context(), c, flags, "auto", entityKebab, false, path, map[string]string{}, nil, cmd.ErrOrStderr())
			if err != nil {
				return classifyAPIError(err, flags)
			}

			var envelope struct {
				Fields []map[string]any `json:"fields"`
			}
			if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Fields) == 0 {
				return apiErr(fmt.Errorf("unexpected entityInformation response for %s: no fields array", entityPascal))
			}

			var picklistFields []string
			var match map[string]any
			for _, f := range envelope.Fields {
				name := strAt(f, "name")
				if isPick, ok := f["isPickList"].(bool); (ok && isPick) || f["picklistValues"] != nil {
					picklistFields = append(picklistFields, name)
				}
				if strings.EqualFold(name, fieldName) {
					match = f
				}
			}
			if match == nil {
				sort.Strings(picklistFields)
				return notFoundErr(fmt.Errorf("field %q not found on %s; picklist fields available: %s", fieldName, entityPascal, strings.Join(picklistFields, ", ")))
			}
			rawValues, _ := match["picklistValues"].([]any)
			if len(rawValues) == 0 {
				sort.Strings(picklistFields)
				return notFoundErr(fmt.Errorf("field %q on %s is not a picklist (no picklistValues); picklist fields available: %s", fieldName, entityPascal, strings.Join(picklistFields, ", ")))
			}
			type value struct {
				Value     string `json:"value"`
				Label     string `json:"label"`
				IsActive  bool   `json:"isActive"`
				IsDefault bool   `json:"isDefault"`
			}
			values := make([]value, 0, len(rawValues))
			for _, rv := range rawValues {
				vm, ok := rv.(map[string]any)
				if !ok {
					continue
				}
				values = append(values, value{
					Value:     strAt(vm, "value"),
					Label:     strAt(vm, "label"),
					IsActive:  boolAt(vm, "isActive"),
					IsDefault: boolAt(vm, "isDefaultValue"),
				})
			}
			out := map[string]any{
				"entity": entityPascal,
				"field":  strAt(match, "name"),
				"source": prov.Source,
				"count":  len(values),
				"values": values,
			}
			return flags.printJSON(cmd, out)
		},
	}
	return cmd
}

// normalizeEntityName accepts "Tickets", "tickets", or "time-entries" and
// returns the PascalCase path segment plus the kebab-case store key.
func normalizeEntityName(arg string) (pascal, kebab string) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", ""
	}
	if strings.Contains(arg, "-") || strings.ToLower(arg) == arg {
		parts := strings.Split(arg, "-")
		for i, p := range parts {
			if p == "" {
				continue
			}
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
		pascal = strings.Join(parts, "")
	} else {
		pascal = strings.ToUpper(arg[:1]) + arg[1:]
	}
	var b strings.Builder
	for i, r := range pascal {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r - 'A' + 'a')
		} else {
			b.WriteRune(r)
		}
	}
	return pascal, b.String()
}
