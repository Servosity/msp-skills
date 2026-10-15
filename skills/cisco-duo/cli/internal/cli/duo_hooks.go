// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (not generated): installs Duo request signing on every client
// (CLI and MCP), the --account-id flag for MSP subaccount routing, and a
// separate local data directory per pinned subaccount.

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"cisco-duo-pp-cli/internal/client"
	"cisco-duo-pp-cli/internal/cliutil"
)

// duoDataDirEnv is the generator's highest-precedence data-dir override.
const duoDataDirEnv = "CISCO_DUO_DATA_DIR"

// duoParentDataDirEnv carries the parent account's data dir to subprocesses
// (MCP shells out to the CLI), so a child never nests subaccounts/<id> twice.
// The PP_ prefix marks it process-internal: an operator never sets it.
const duoParentDataDirEnv = "PP_CISCO_DUO_PARENT_DATA_DIR"

func init() {
	// A pin from the environment (e.g. an MCP server config) isolates the
	// store before anything resolves a database path.
	if id := client.PinnedAccountID(); id != "" {
		_ = pinSubaccountDataDir(id)
	}
	client.AccountResolvedHook = bindNameStore
	client.UnpinnedAccountGuard = refuseUnpinnedRequestAccount
	registerClientHook(func(c *client.Client) error {
		c.HTTPClient.Transport = client.NewDuoSigner(c.HTTPClient.Transport)
		// The generated response cache keys on the request as built, before the
		// signer adds a pinned account_id or turns a subaccount NAME into its ID,
		// so it could serve a parent's, another subaccount's or a reassigned
		// name's response. Never cache in this multi-tenant connector.
		// ponytail: costs repeat GETs within one run; key the cache on the routed
		// account ID if that ever matters.
		c.NoCache = true
		return nil
	})
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if root.PersistentFlags().Lookup("account-id") == nil {
			root.PersistentFlags().Var(accountEnvFlag{}, "account-id", accountFlagUsage)
		}
		// refuseSyncParams rejects every use of these, so stop advertising them
		// (help, agent-context and the MCP sync tool schema all skip hidden flags).
		if syncCmd, _, err := root.Find([]string{"sync"}); err == nil && syncCmd != root {
			for _, name := range []string{"param", "global-param", "resource-param"} {
				_ = syncCmd.Flags().MarkHidden(name)
			}
		}
		pinRequestAccountFlags(root)
		prev := root.PersistentPreRunE
		root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			// Every CLI command may write live responses into (or answer from)
			// the local store, so an unpinned request may not name a
			// subaccount; the fleet fan-out, which never touches the store,
			// lifts this in duoFleetPrep. The MCP server never runs this
			// pre-run: its execute tool keeps nothing locally.
			storeScopedRun.Store(true)
			// --home is applied inside the generated pre-run, after flags were
			// parsed; apply it first so a pinned subaccount store lands under it.
			if id := currentAccountID(); id != "" && flags.homePath != "" {
				if _, err := cliutil.SetHomeOverride(flags.homePath); err != nil {
					return err
				}
				if err := pinSubaccountDataDir(id); err != nil {
					return err
				}
			}
			if prev != nil {
				if err := prev(cmd, args); err != nil {
					return err
				}
			}
			// After the generated pre-run, which applies a saved --profile's
			// values, so a profile cannot carry an account_id sync param (or a
			// pin) past these checks.
			if err := refuseParentWriteUnderPin(cmd); err != nil {
				return err
			}
			return refuseSyncParams(cmd)
		}
	})
}

// storeScopedRun is set while a CLI command runs (the root pre-run) and
// cleared by duoFleetPrep: it tells refuseUnpinnedRequestAccount that this
// process keeps live responses in the local store.
var storeScopedRun atomic.Bool

// refuseUnpinnedRequestAccount is the client's UnpinnedAccountGuard. With
// nothing pinned, the local store is the parent account's, so a CLI request
// that names a subaccount of its own (an account_id in a --stdin body or an
// import line) would write that customer's data into the parent's mirror.
func refuseUnpinnedRequestAccount(requested string) error {
	if !storeScopedRun.Load() {
		return nil
	}
	return fmt.Errorf("the request names subaccount %q in its own account_id, but no subaccount is pinned, so its response would be stored as the parent account's data; run it with --account-id %s (or %s) and leave account_id out of the request", requested, shellQuote(requested), client.DuoAccountEnv)
}

// pinningAccountFlag wraps a generated command's own --account-id (the
// billing commands send it as the request's account_id, shadowing the root
// flag) so that setting it also pins the run to that subaccount: the request
// and the local store then name the same customer, instead of the request
// reaching the subaccount while its response is stored in (or a local
// fallback read answers from) the parent's or another pinned customer's store.
type pinningAccountFlag struct{ pflag.Value }

func (f pinningAccountFlag) Set(v string) error {
	if err := f.Value.Set(v); err != nil {
		return err
	}
	return accountEnvFlag{}.Set(v)
}

// pinRequestAccountFlags installs pinningAccountFlag on every command that
// declares its own --account-id, except the /accounts/v1/* commands, which
// always run against the MSP parent (accounts delete names the subaccount
// to remove, not one to run as).
func pinRequestAccountFlags(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		if f := child.Flags().Lookup("account-id"); f != nil && !strings.HasPrefix(child.Annotations["pp:path"], "/accounts/") {
			switch f.Value.(type) {
			case accountEnvFlag, pinningAccountFlag:
			default:
				f.Value = pinningAccountFlag{f.Value}
			}
		}
		pinRequestAccountFlags(child)
	}
}

// refuseParentWriteUnderPin stops `accounts create` / `accounts delete` while
// a subaccount is pinned: they run against the MSP parent (the signer never
// routes /accounts/v1/*), and their response, a parent-level row describing
// some other customer's subaccount, would be stored in the pinned customer's
// mirror. `accounts list` stores nothing, so it is unaffected.
func refuseParentWriteUnderPin(cmd *cobra.Command) error {
	pinned := currentAccountID()
	if pinned == "" || !strings.HasPrefix(cmd.Annotations["pp:path"], "/accounts/") || cmd.Annotations["mcp:read-only"] == "true" {
		return nil
	}
	return usageErr(fmt.Errorf("%s manages subaccounts from the MSP parent account, but subaccount %q is pinned (--account-id or %s), whose local store would receive the parent's response; run it without a pin", cmd.CommandPath(), pinned, client.DuoAccountEnv))
}

var (
	// userDataDir is the user's own CISCO_DUO_DATA_DIR, captured before this
	// file ever overrides it; inheritedBase is a base handed down by a
	// parent process that already pinned a subaccount.
	userDataDir     = strings.TrimSpace(os.Getenv(duoDataDirEnv))
	inheritedBase   = strings.TrimSpace(os.Getenv(duoParentDataDirEnv))
	unsafePathChars = regexp.MustCompile(`[^a-z0-9._-]+`)
)

// subaccountDirName keys the store directory by account ID (upper-cased) or,
// for a name, by a readable slug plus a hash of the case-folded name, so
// distinct names never share a directory. Use the ID for one stable store.
func subaccountDirName(idOrName string) string {
	v := strings.TrimSpace(idOrName)
	if duoAccountIDRE.MatchString(strings.ToUpper(v)) {
		return strings.ToUpper(v)
	}
	folded := strings.ToLower(v)
	slug := strings.Trim(unsafePathChars.ReplaceAllString(folded, "-"), "-.")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	sum := sha256.Sum256([]byte(folded))
	return "name-" + slug + "-" + hex.EncodeToString(sum[:])[:12]
}

// pinSubaccountDataDir points the local store (sync, search, audits, MCP
// search) at <data dir>/subaccounts/<account>, so each customer's mirror is
// a separate directory and the generated legacy-store adoption can never see
// another tenant's files. The base is the parent's data dir as the user
// configured it (their own CISCO_DUO_DATA_DIR, --home, XDG or default).
func pinSubaccountDataDir(idOrName string) error {
	base := inheritedBase
	if base == "" {
		if userDataDir != "" {
			if err := os.Setenv(duoDataDirEnv, userDataDir); err != nil {
				return err
			}
		} else if err := os.Unsetenv(duoDataDirEnv); err != nil {
			return err
		}
		resolved, err := cliutil.DataDir()
		if err != nil {
			return err
		}
		base = resolved
		if err := os.Setenv(duoParentDataDirEnv, base); err != nil {
			return err
		}
	}
	if strings.TrimSpace(idOrName) == "" {
		return os.Setenv(duoDataDirEnv, base)
	}
	return os.Setenv(duoDataDirEnv, filepath.Join(base, "subaccounts", subaccountDirName(idOrName)))
}

// accountFlagUsage is shared by the root flag and the novel commands that
// declare it locally (so SKILL/README flag checks see it on those commands).
const accountFlagUsage = "Run against this MSP subaccount (ID or exact name from 'accounts list'); requests go to that subaccount's API host and use its own local store (env: " + client.DuoAccountEnv + ")"

// accountFlagSink is the declared storage of the local --account-id flag on
// novel commands; useAccountFlag swaps in accountEnvFlag as its Value.
var accountFlagSink string

// useAccountFlag declares --account-id on a novel command. Its Value is
// accountEnvFlag, so command-line and saved-profile values (which call
// Value.Set without marking the flag changed) both route the request.
func useAccountFlag(cmd *cobra.Command) {
	cmd.Flags().StringVar(&accountFlagSink, "account-id", "", accountFlagUsage)
	cmd.Flags().Lookup("account-id").Value = accountEnvFlag{}
}

// accountEnvFlag stores --account-id in CISCO_DUO_ACCOUNT_ID so the signer
// routes every request (generated commands included) to that subaccount,
// and isolates that subaccount's local store.
type accountEnvFlag struct{}

func (accountEnvFlag) String() string { return client.PinnedAccountID() }
func (accountEnvFlag) Type() string   { return "string" }
func (accountEnvFlag) Set(v string) error {
	v = strings.TrimSpace(v)
	if err := os.Setenv(client.DuoAccountEnv, v); err != nil {
		return err
	}
	return pinSubaccountDataDir(v)
}

// bindNameStore ties a pinned subaccount's local store to the account ID the
// pin first resolved to, and refuses it once the pin resolves to a different
// subaccount (a name renamed or reassigned, or an ID whose account was deleted
// and whose string is now another account's name), so one customer's mirror
// is never synced into or read as another's.
// ponytail: a local-only read sends no request, so it cannot notice the move
// until the next live call; pin by account ID to rule that out entirely.
func bindNameStore(requested, accountID string) error {
	name := strings.TrimSpace(requested)
	if name == "" || !strings.EqualFold(name, currentAccountID()) {
		return nil
	}
	dir, err := cliutil.DataDir()
	if err != nil || filepath.Base(dir) != subaccountDirName(name) {
		return nil
	}
	marker := filepath.Join(dir, "account-id")
	got, _, err := cliutil.ReadFileWithLegacyFallback(marker, "")
	if errors.Is(err, os.ErrNotExist) {
		if err := claimStoreMarker(dir, marker, accountID); err != nil {
			return err
		}
		got, _, err = cliutil.ReadFileWithLegacyFallback(marker, "")
	}
	if err != nil {
		return err
	}
	was := strings.TrimSpace(string(got))
	// The no-hard-link fallback creates the marker before writing it: give a
	// concurrent claimant a moment to finish. A marker still empty after that
	// is an interrupted claim; it is never taken over (a delayed claimant could
	// still be writing another account's ID), so the store stays refused.
	for i := 0; was == "" && i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		if got, _, err = cliutil.ReadFileWithLegacyFallback(marker, ""); err != nil {
			return err
		}
		was = strings.TrimSpace(string(got))
	}
	if was == "" {
		// The directory may still hold synced rows (a crash can leave the
		// unsynced marker empty), so the advice is the directory, never the marker.
		return fmt.Errorf("subaccount %q: its local store %s has an empty account-id marker (an interrupted first run); delete that directory and re-run, or pass --account-id %s", name, dir, accountID)
	}
	if was != accountID {
		return fmt.Errorf("subaccount name %q now resolves to %s, but its local store %s holds data for %s; pass --account-id %s (each account ID gets its own store) or delete that directory", name, accountID, dir, was, accountID)
	}
	return nil
}

// claimStoreMarker publishes the store's account-id marker with a hard link,
// which succeeds for exactly one writer: two first runs racing on one store
// cannot both claim it, and the loser then reads (and is checked against)
// the winner's marker.
func claimStoreMarker(dir, marker, accountID string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".account-id-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, werr := tmp.WriteString(accountID + "\n")
	if err := errors.Join(werr, tmp.Close()); err != nil {
		return err
	}
	err = os.Link(tmp.Name(), marker)
	if err == nil || errors.Is(err, os.ErrExist) {
		return nil
	}
	// No hard links on this filesystem (exFAT, FAT, some network mounts):
	// an exclusive create still lets only one claimant win.
	f, err := os.OpenFile(filepath.Clean(marker), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, werr = f.WriteString(accountID + "\n")
	return errors.Join(werr, f.Close())
}

// refuseSyncParams stops `sync --param account_id=...` (and the
// --global-param / --resource-param <resource>:account_id=... forms): the
// request would fetch that subaccount while writing into the current (parent
// or pinned) store. --account-id routes the sync AND gives it its own store.
// Every other parameter is refused too: sync marks what it fetched as
// complete and the audits trust that, so a filter (username=...) or a page
// size the paginator does not expect (limit=1 ends paging after one page)
// would leave a partial mirror that reports a tenant as clean.
func refuseSyncParams(cmd *cobra.Command) error {
	if cmd.Name() != "sync" || cmd.Parent() != cmd.Root() {
		return nil
	}
	for _, name := range []string{"param", "global-param", "resource-param"} {
		vals, err := cmd.Flags().GetStringArray(name)
		if err != nil {
			continue
		}
		for _, kv := range vals {
			key, _, _ := strings.Cut(kv, "=")
			if i := strings.LastIndex(key, ":"); i >= 0 { // resource:key=value
				key = key[i+1:]
			}
			switch k := strings.ToLower(strings.TrimSpace(key)); k {
			case "account_id":
				return usageErr(fmt.Errorf("--%s %s: pass the subaccount with --account-id instead, which also gives it its own local store; an account_id sync parameter would write that customer's data into the current store", name, kv))
			default:
				return usageErr(fmt.Errorf("--%s %s: sync builds the complete local mirror the audits rely on and takes no extra parameters; a %q parameter could leave it partial while marking it complete. Filter with the live list commands instead", name, kv, k))
			}
		}
	}
	return nil
}
