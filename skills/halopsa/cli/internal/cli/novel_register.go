// Hand-written novel feature registration. Not generated.
// Wires the transcendence commands into the root command tree with explicit
// AddCommand calls so dogfood's static wiring check can see them. Children of
// spec-derived resources (tickets, sla, agent, asset, kbarticle) are wired by
// the generated resource files themselves; this hub owns only the top-level
// novel groups.
package cli

import "github.com/spf13/cobra"

// registerNovelFeatures adds the top-level transcendence commands plus the
// hand-written SELECT-only `sql` command over the local store (this tree's
// generator emits no framework sql equivalent).
func registerNovelFeatures(rootCmd *cobra.Command, flags *rootFlags) {
	rootCmd.AddCommand(newTriageCmd(flags))
	rootCmd.AddCommand(newSQLCmd(flags))
	rootCmd.AddCommand(newStandupCmd(flags))

	timeCmd := newTimeCmd(flags)
	timeCmd.AddCommand(newNovelTimeLeaksCmd(flags))
	rootCmd.AddCommand(timeCmd)

	rootCmd.AddCommand(newContractsCmd(flags))
	rootCmd.AddCommand(newRulesCmd(flags))

	// assets group (generated assets.go wires the expiring child).
	rootCmd.AddCommand(newNovelAssetsCmd(flags))

	// client group: card + overlay keep their prior `client <sub>` command
	// paths. The /Client resource family is exposed as `clients` (renamed via
	// x-pp-resource to avoid the reserved framework template), so this small
	// hand group owns the per-client composite views.
	clientCmd := &cobra.Command{
		Use:         "client",
		Short:       "Per-client composite views (card, overlay)",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	clientCmd.AddCommand(newClientCardCmd(flags))
	clientCmd.AddCommand(newClientOverlayCmd(flags))
	rootCmd.AddCommand(clientCmd)

	// Unhide spec-derived parents that carry novel children so the novel
	// surface is discoverable from --help.
	for _, name := range []string{"tickets", "sla", "agent", "asset", "kbarticle"} {
		if c := findChildCmd(rootCmd, name); c != nil {
			c.Hidden = false
		}
	}
}

func findChildCmd(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}
