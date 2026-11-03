// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Friendly noun-verb command layer.
//
// Auvik's OpenAPI documents name operations like `readMultipleDeviceInfo` and
// group them by URL segment, so the generated surface reads
// `auvik-cli inventory read-multiple-device-info`. That is faithful to the
// spec and unusable as a daily driver.
//
// This file adds noun-verb doors over the highest-traffic reads:
// `auvik-cli device list`. Each alias is a FRESH instance built by the same
// generated constructor, so it carries every flag, every enum hint, and the same
// RunE as the endpoint command it mirrors. The verbose endpoint commands remain
// and remain the complete surface.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		attachFriendlyLayer(root, flags)
	})
}

// alias rebuilds a generated command under a friendlier name.
//
// Three things here are load-bearing and were each a real bug:
//
//  1. POSITIONALS ARE PRESERVED. The generated Use is e.g.
//     "read-single-device-info <id>". Rewriting it to bare "get" drops the
//     positional, and internal/mcp/cobratree derives every MCP tool parameter by
//     parsing cmd.Use — so the mirrored `device get` tool would expose NO
//     parameter for the id an agent must pass. The tail is carried over.
//
//  2. INHERITED ALIASES ARE CLEARED. The generator assigns Aliases like
//     ["list"]/["get"]/["create"] to the first matching endpoint in a group.
//     Cloning `alert dismiss-single` (Aliases: ["create"]) into `alert dismiss`
//     without clearing them left `alert create` as a live spelling that DISMISSES
//     an alert. An alias is only meaningful on the command the generator put it
//     on, never on a clone under a different name.
//
//  3. THE EXAMPLE IS RETARGETED WITH THE PARENT PASSED IN, not looked up from a
//     package-level map. The map version was written after alias() had already
//     run and was keyed by leaf only, so every `list` alias collided and each
//     command advertised a sibling's example.
func alias(src *cobra.Command, parent, use, short string) *cobra.Command {
	origUse := src.Use
	origName := src.Name()

	// Carry any positional tail: "read-single-device-info <id>" -> "get <id>".
	if i := strings.IndexByte(origUse, ' '); i >= 0 {
		src.Use = use + origUse[i:]
	} else {
		src.Use = use
	}

	// A clone must not inherit the original's alias spellings.
	src.Aliases = nil

	if short != "" {
		src.Short = short
	}
	if src.Example != "" {
		src.Example = retargetExample(src.Example, origName, parent, use)
	}
	if src.Annotations == nil {
		src.Annotations = map[string]string{}
	}
	src.Annotations["pp:friendly-alias"] = "true"
	return src
}

// retargetExample rewrites the command path inside a generated Example so the
// alias does not advertise the verbose path it was cloned from. Only lines whose
// path actually matches the source command are rewritten; anything else (a
// `sync` line, a piped shell command) is left alone.
func retargetExample(example, origName, parent, use string) string {
	const prefix = "auvik-cli "
	leaf := use
	if i := strings.IndexByte(leaf, ' '); i >= 0 {
		leaf = leaf[:i]
	}

	lines := strings.Split(example, "\n")
	for i, line := range lines {
		idx := strings.Index(line, prefix)
		if idx < 0 {
			continue
		}
		head := line[:idx+len(prefix)]
		fields := strings.Fields(line[idx+len(prefix):])
		if len(fields) < 2 {
			continue
		}
		// Generated paths are "<group> <operation> [args...]". Only rewrite when
		// the operation is the command we actually cloned.
		if fields[1] != origName {
			continue
		}
		rest := fields[2:]
		rebuilt := head + parent + " " + leaf
		if len(rest) > 0 {
			rebuilt += " " + strings.Join(rest, " ")
		}
		lines[i] = strings.TrimRight(rebuilt, " ")
	}
	return strings.Join(lines, "\n")
}

func attachFriendlyLayer(root *cobra.Command, flags *rootFlags) {
	// This hook runs BEFORE root.go attaches the novel parent stubs
	// (device/configuration/usage). Creating a bare group of the same name first
	// would make root.go's addNovelCommandIfAbsent skip the real stub as a
	// duplicate and silently drop the transcendence command it carries. So for
	// any group a novel stub owns, seed it FROM that stub.
	device := findOrCreateGroup(root, "device",
		"Devices: inventory, details, lifecycle, warranty, and discovery health",
		func() *cobra.Command { return newNovelDeviceCmd(flags) })
	configuration := findOrCreateGroup(root, "configuration",
		"Device configuration backups and fleet-wide backup-coverage audit",
		func() *cobra.Command { return newNovelConfigurationCmd(flags) })
	usage := findOrCreateGroup(root, "usage",
		"Billable usage and invoice reconciliation",
		func() *cobra.Command { return newNovelUsageCmd(flags) })

	// These already exist in the generated tree, or have no novel counterpart.
	alert := findOrCreateGroup(root, "alert", "Alerts: history, triage, and dismissal", nil)
	tenants := findOrCreateGroup(root, "tenants", "Clients and multi-clients you can see", nil)
	network := findOrCreateGroup(root, "network", "Discovered networks", nil)
	iface := findOrCreateGroup(root, "interface", "Device interfaces", nil)

	add := func(parent *cobra.Command, leaf, short string, build func() *cobra.Command) {
		// The generator already declares Cobra Aliases like "list"/"get" on the
		// first list/get endpoint in each group. Leaving those in place while we
		// add a real command of the same name means two siblings claim one word
		// and Cobra resolves to whichever registered first. Release the alias so
		// exactly one command owns the name.
		releaseSiblingAlias(parent, leaf)
		addNovelCommandIfAbsent(parent, alias(build(), parent.Name(), leaf, short))
	}

	// device
	add(device, "list", "List discovered devices across your clients",
		func() *cobra.Command { return newInventoryReadMultipleDeviceInfoCmd(flags) })
	add(device, "get", "Get one device by id",
		func() *cobra.Command { return newInventoryReadSingleDeviceInfoCmd(flags) })
	add(device, "detail", "Extra collected detail for many devices",
		func() *cobra.Command { return newInventoryReadMultipleDeviceDetailsCmd(flags) })
	add(device, "lifecycle", "Hardware lifecycle status (feeds 'eol')",
		func() *cobra.Command { return newInventoryReadMultipleDeviceLifecycleCmd(flags) })
	add(device, "warranty", "Warranty coverage and expiry (feeds 'eol')",
		func() *cobra.Command { return newInventoryReadMultipleDeviceWarrantyCmd(flags) })

	// network / interface
	add(network, "list", "List discovered networks",
		func() *cobra.Command { return newInventoryReadMultipleNetworkInfoCmd(flags) })
	add(iface, "list", "List device interfaces",
		func() *cobra.Command { return newInventoryReadMultipleInterfaceInfoCmd(flags) })

	// alert
	add(alert, "list", "List alert history",
		func() *cobra.Command { return newAlertReadMultipleInfoCmd(flags) })
	add(alert, "get", "Get one alert by id",
		func() *cobra.Command { return newAlertReadSingleInfoCmd(flags) })
	add(alert, "dismiss", "Dismiss an alert (the only write this API supports)",
		func() *cobra.Command { return newAlertDismissSingleCmd(flags) })

	// tenants
	add(tenants, "list", "List clients and multi-clients you can access",
		func() *cobra.Command { return newTenantsReadMultipleCmd(flags) })

	// configuration
	add(configuration, "list", "List stored configuration backups",
		func() *cobra.Command { return newInventoryReadMultipleConfigurationsCmd(flags) })
	add(configuration, "get", "Get one configuration backup record by id",
		func() *cobra.Command { return newInventoryReadSingleConfigurationCmd(flags) })

	// usage
	add(usage, "client", "Billable usage summary for a client",
		func() *cobra.Command { return newBillingReadClientUsageCmd(flags) })
	add(usage, "device", "Billable usage for one device",
		func() *cobra.Command { return newBillingReadDeviceUsageCmd(flags) })
}

// releaseSiblingAlias drops `name` from every existing child's Aliases so a
// newly added command of that name is the sole claimant. The sibling keeps its
// canonical name and stays fully reachable.
func releaseSiblingAlias(parent *cobra.Command, name string) {
	for _, sib := range parent.Commands() {
		if len(sib.Aliases) == 0 {
			continue
		}
		kept := sib.Aliases[:0]
		for _, a := range sib.Aliases {
			if a != name {
				kept = append(kept, a)
			}
		}
		sib.Aliases = kept
	}
}

// findOrCreateGroup returns the existing top-level group with this name. When
// absent, it attaches one built by seed (so a novel parent keeps the children it
// already carries) or a bare group when seed is nil.
func findOrCreateGroup(root *cobra.Command, name, short string, seed func() *cobra.Command) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	var grp *cobra.Command
	if seed != nil {
		grp = seed()
	}
	if grp == nil {
		grp = &cobra.Command{
			Use:         name,
			Annotations: map[string]string{"mcp:read-only": "true"},
			RunE: func(cmd *cobra.Command, args []string) error {
				return cmd.Help()
			},
		}
	}
	grp.Use = name
	grp.Short = short
	root.AddCommand(grp)
	return grp
}
