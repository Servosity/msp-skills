// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: devices locked.
// pp:data-source local

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"apple-business-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type lockedRow struct {
	Org        string `json:"org,omitempty"`
	Serial     string `json:"serial"`
	Model      string `json:"model"`
	Family     string `json:"family"`
	LockType   string `json:"lock_type"`
	ServerName string `json:"server_name,omitempty"`
	OwnerName  string `json:"owner_name,omitempty"`
	OwnerEmail string `json:"owner_email,omitempty"`
	CheckedAt  string `json:"checked_at"`
}

func newNovelDevicesLockedCmd(flags *rootFlags) *cobra.Command {
	var lockType, dbPath string
	var allOrgs bool
	cmd := &cobra.Command{
		Use:   "locked",
		Short: "Devices currently Activation Locked (user locks by default) with the owner to contact when known",
		Long: `Use this command to list devices currently Activation Locked (user locks by default) with the owner when known. Do NOT use this command for one device's lock state; use 'devices lookup' instead.

Reads Activation Lock data stored by 'devices enrich --activation-lock'. A USER
lock needs the previous owner's Apple Account; an MDM lock can be cleared by the
device management service. The owner comes from Apple's built-in device
management (mdmDevices.enrolledUserId joined to users) when that data is synced.`,
		Example: strings.Trim(`
  apple-business-cli devices locked --type user --agent
  apple-business-cli devices locked --type any --all-orgs --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "devices locked")
			}
			want := strings.ToUpper(lockType)
			if want != "USER" && want != "MDM" && want != "ANY" {
				return usageErr(fmt.Errorf("--type must be user, mdm or any"))
			}
			paths := orgStorePaths(dbPath, allOrgs)
			multi := len(paths) > 1 || allOrgs
			rows := make([]lockedRow, 0)
			notEnriched := 0
			for _, path := range paths {
				if _, err := os.Stat(path); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "no local store at %s\nrun: apple-business-cli sync --resources devices && apple-business-cli devices enrich --activation-lock\n", path)
					continue
				}
				got, missing, err := lockedRowsFrom(cmd, flags, path, multi, want)
				if err != nil {
					return err
				}
				rows = append(rows, got...)
				notEnriched += missing
			}
			if notEnriched > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: %d device(s) have no Activation Lock data; run 'apple-business-cli devices enrich --activation-lock'\n", notEnriched)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No Activation Locked devices in the local store.")
				return nil
			}
			for _, r := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s  %s  %s  lock=%s  server=%s  owner=%s %s\n", prefixOrg(r.Org), r.Serial, r.Family, r.Model, r.LockType, firstNonEmpty(r.ServerName, "-"), firstNonEmpty(r.OwnerName, "-"), r.OwnerEmail)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&lockType, "type", "user", "Lock type to list: user, mdm or any")
	cmd.Flags().BoolVar(&allOrgs, "all-orgs", false, "Read every customer org's local store, not just the active API account's")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the active API account's store)")
	return cmd
}

func lockedRowsFrom(cmd *cobra.Command, flags *rootFlags, path string, multi bool, want string) ([]lockedRow, int, error) {
	ctx := cmd.Context()
	db, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, 0, fmt.Errorf("opening %s: %w", path, err)
	}
	defer db.Close()
	if !multi && !hintIfUnsynced(cmd, db, "devices") {
		hintIfStale(cmd, db, "devices", flags.maxAge)
	}
	devices, err := loadLocalDevices(ctx, db)
	if err != nil {
		return nil, 0, err
	}
	locks, err := loadEnrichment(ctx, db, "lock")
	if err != nil {
		return nil, 0, err
	}
	links, _ := loadEnrichment(ctx, db, "server")
	servers, _ := loadLocalServers(ctx, db)
	serverName := map[string]string{}
	for _, s := range servers {
		serverName[s.ID] = s.Attributes.ServerName
	}
	owners := ownerIndex(ctx, db)
	label := ""
	if multi {
		if path != storeDBPath(flagDB(cmd)) && !storeHasData(ctx, db) {
			return nil, 0, nil
		}
		label = orgLabel(ctx, db, path)
		hintStoreUnsynced(cmd, db, label)
	}
	rows := make([]lockedRow, 0)
	missing := 0
	for _, d := range devices {
		if d.released() {
			continue
		}
		e, ok := locks[d.serial()]
		if !ok {
			missing++
			continue
		}
		l, ok := lockFrom(e)
		if !ok || !l.IsLocked {
			continue
		}
		if want != "ANY" && !strings.EqualFold(l.LockType, want) {
			continue
		}
		r := lockedRow{Org: label, Serial: d.serial(), Model: d.Attributes.DeviceModel, Family: d.Attributes.ProductFamily, LockType: l.LockType, CheckedAt: e.EnrichedAt.Format(time.RFC3339)}
		if link, ok := links[d.serial()]; ok {
			r.ServerName = serverName[serverLinkID(link)]
		}
		if o, ok := owners[d.serial()]; ok {
			r.OwnerName, r.OwnerEmail = o[0], o[1]
		}
		rows = append(rows, r)
	}
	return rows, missing, nil
}

// ownerIndex maps serial -> [name, email] via mdmDevices.enrolledUserId and
// users; empty when built-in device management data is not synced.
func ownerIndex(ctx context.Context, db *store.Store) map[string][2]string {
	out := map[string][2]string{}
	mdm, err := loadResourceRows(ctx, db, "mdm_devices")
	if err != nil || len(mdm) == 0 {
		return out
	}
	users, _ := loadResourceRows(ctx, db, "users")
	byID := map[string][2]string{}
	for _, u := range users {
		var v struct {
			ID         string `json:"id"`
			Attributes struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
				Email     string `json:"email"`
				Managed   string `json:"managedAppleAccount"`
			} `json:"attributes"`
		}
		if json.Unmarshal(u, &v) == nil {
			byID[v.ID] = [2]string{strings.TrimSpace(v.Attributes.FirstName + " " + v.Attributes.LastName), firstNonEmpty(v.Attributes.Email, v.Attributes.Managed)}
		}
	}
	for _, m := range mdm {
		var v struct {
			Attributes struct {
				SerialNumber   string `json:"serialNumber"`
				EnrolledUserID string `json:"enrolledUserId"`
			} `json:"attributes"`
		}
		if json.Unmarshal(m, &v) == nil && v.Attributes.EnrolledUserID != "" {
			if o, ok := byID[v.Attributes.EnrolledUserID]; ok {
				out[v.Attributes.SerialNumber] = o
			}
		}
	}
	return out
}
