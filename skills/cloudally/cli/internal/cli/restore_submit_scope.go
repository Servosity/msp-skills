// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"io"
	"strings"
)

// Auth scope labels recorded on a restore batch.
const (
	batchAuthPartner      = "partner"
	batchAuthAccountLogin = "account-login"
	batchAuthUseAccount   = "use-account"
)

// unrecordedAccountLabel is shown when no auth context names the account.
const unrecordedAccountLabel = "(not recorded: no auth context; account token from config or environment)"

// submitAccount is the customer a batch restores into, as recorded on the
// batch: the --account id, or the account the active login belongs to.
type submitAccount struct {
	id        string
	authScope string
}

func (a submitAccount) label() string {
	switch {
	case a.id != "" && a.authScope != "":
		return a.id + " (" + a.authScope + ")"
	case a.id != "":
		return a.id
	case a.authScope == batchAuthAccountLogin:
		return "(the company signed in with auth login; id not known)"
	default:
		return unrecordedAccountLabel
	}
}

// errPartnerNeedsAccount is returned when a partner token would be used on
// the account-scoped restore endpoints.
var errPartnerNeedsAccount = errors.New("account-scoped restore endpoints need an account token; pass --account <id> or run `auth use-account <id>` / `auth login` as the company")

// resolveSubmitAccount decides which customer account a submit is for.
// --account wins. Without it the auth sidecar names the active scope: a
// use-account token records its account id, a company login records the
// scope only, and a partner token is refused because the account-scoped
// endpoints would act on no particular customer. A missing sidecar (token
// supplied through config or the environment) records nothing.
func resolveSubmitAccount(scope restoreScope) (submitAccount, error) {
	actx, ok := loadAuthContext()
	if scope.account != "" {
		a := submitAccount{id: scope.account, authScope: batchAuthPartner}
		if ok && actx.Scope == authScopeAccount {
			a.authScope = batchAuthUseAccount
		}
		return a, nil
	}
	if !ok {
		return submitAccount{}, nil
	}
	switch actx.Scope {
	case authScopePartner:
		return submitAccount{}, usageErr(errPartnerNeedsAccount)
	case authScopeAccount:
		if id := strings.TrimSpace(actx.AccountID); id != "" {
			return submitAccount{id: id, authScope: batchAuthUseAccount}, nil
		}
		return submitAccount{authScope: batchAuthAccountLogin}, nil
	}
	return submitAccount{}, nil
}

// printSubmitConfirmation writes the plan summary shown when --yes is
// missing.
func printSubmitConfirmation(w io.Writer, req submitRequest) {
	entities := map[string]bool{}
	for _, r := range req.rows {
		entities[strings.ToLower(strings.TrimSpace(r.Entity))] = true
	}
	termf(w, "restore submit would send %d restore(s); nothing has been sent\n", len(req.rows))
	termf(w, "  rows:     %d\n", len(req.rows))
	termf(w, "  entities: %d\n", len(entities))
	termf(w, "  dates:    %s (cutoffs in UTC)\n", strings.Join(req.dateTexts, ", "))
	termf(w, "  scope:    %s (POST %s)\n", req.scope.kind(), req.restorePath)
	termf(w, "  account:  %s\n", req.account.label())
	termf(w, "  task:     %s (%s)\n", req.taskID, req.source)
	if req.resume {
		termf(w, "  resume:   rows already in the batch ledger keep their status; only PLANNED and FAILED rows the API never accepted would be sent, with the target account and restore options recorded on the batch\n")
	}
	shown := req.rows
	const maxShown = 20
	if len(shown) > maxShown {
		shown = shown[:maxShown]
	}
	for _, r := range shown {
		termf(w, "    %s  requested %s  -> backup %s\n", r.Entity, r.Requested, r.Resolved)
	}
	if len(req.rows) > len(shown) {
		termf(w, "    ... and %d more\n", len(req.rows)-len(shown))
	}
}
