// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package keeper is the only path from this CLI to Keeper Commander Service
// Mode's command endpoint (POST /api/v1/executecommand). keeper handfix
// commander-allowlist:
//
//   - The allowlist is the whole surface. A Command can only be built by the
//     constructors in this file; each one emits a fixed Commander read-only
//     report with fixed flags. There is no constructor that takes a command
//     string, so nothing (a flag, an MCP argument, a config value) can send a
//     command outside this file.
//   - Every value an operator supplies (a managed company id, a day count, a
//     month, an email, an event type) is matched against a strict pattern and
//     becomes exactly one token. The accepted characters cannot quote, escape,
//     expand or separate anything in Service Mode's shlex tokenizer
//     (keepercommander/service/util/verified_command.py), and no value may
//     start with '-', so a value can never become a flag.
//   - Nothing here reads vault record contents, passwords, attachments,
//     shared-folder secrets or one-time shares, and nothing writes: write and
//     host-file flags of these reports (--output, --save, --attempt-fix,
//     --record-details, --apply-action) are never emitted. action-report,
//     compliance, export, get and search are not on the list.
//   - Each Command carries the output fields it may show; Run projects every
//     row onto them (output.go), so a field Keeper adds or returns
//     unexpectedly is dropped, never printed.
package keeper

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Command is one allowlisted Commander invocation. The zero value is invalid.
type Command struct {
	name   string
	args   []string
	fields []string            // the row fields this command may show; nil for context switches
	nested map[string][]string // object fields kept, with the only keys they may show
	kind   kind
}

type kind int

const (
	kindRows    kind = iota + 1 // --format=json report: a JSON array of objects
	kindContext                 // switch-to-mc / switch-to-msp: a log message
)

// AllowedCommands is the first token of every command this CLI can send.
// Service Mode's own per-key allowlist (service-create -c) should be set to
// exactly this list (README "Defense in depth").
var AllowedCommands = []string{
	"msp-info",
	"msp-billing-report",
	"enterprise-info",
	"user-report",
	"security-audit-report",
	"audit-report",
	"switch-to-mc",
	"switch-to-msp",
}

func allowedName(name string) bool {
	for _, n := range AllowedCommands {
		if n == name {
			return true
		}
	}
	return false
}

// String is the exact command text sent to Service Mode.
func (c Command) String() string {
	return strings.Join(append([]string{c.name}, c.args...), " ")
}

// Name is the Commander command (the Service Mode allowlist key).
func (c Command) Name() string { return c.name }

// Fields returns a copy of the output field allowlist.
func (c Command) Fields() []string { return append([]string(nil), c.fields...) }

// safeToken is the whole character set a command token may use. It has no
// whitespace, quote, backslash, '$', '#', ';', '|', '&', '<', '>', '(', ')' or
// control byte, so shlex returns each token unchanged and html.escape (applied
// by Service Mode) leaves it unchanged.
var safeToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+%,=:-]*$`)

// validate is the last check before a request: the command must come from a
// constructor (known name, kind set) and every token must be safe.
func (c Command) validate() error {
	if c.kind == 0 || !allowedName(c.name) {
		return fmt.Errorf("refused: %q is not on this CLI's read-only Commander allowlist", c.name)
	}
	for _, a := range c.args {
		if a == "--" || !(safeToken.MatchString(a) || safeFlag.MatchString(a)) {
			return fmt.Errorf("refused: unsafe token in %s", c.name)
		}
	}
	return nil
}

// safeFlag is a long flag this file emits (--name or --name=value).
var safeFlag = regexp.MustCompile(`^--[a-z][a-z-]*(=[A-Za-z0-9._@+%,:-]+)?$`)

// Value patterns. Each is anchored and length-bounded.
var (
	reCompanyID = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
	reMonth     = regexp.MustCompile(`^(19|20)[0-9]{2}-(0[1-9]|1[0-2])$`)
	reEmail     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._%+-]{0,63}@[A-Za-z0-9](?:[A-Za-z0-9-]{0,62}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,62}[A-Za-z0-9])?)+$`)
	reEventType = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
)

// CompanyID validates a managed company id (msp-info company_id). Ids only:
// Commander resolves an all-digit argument by id and anything else by name
// (commands/msp.py get_mc_by_name_or_id), so a name is never sent.
func CompanyID(raw string) (string, error) {
	// No trimming: a value is accepted exactly as given or refused.
	if !reCompanyID.MatchString(raw) {
		return "", fmt.Errorf("invalid managed company id %q: use the numeric company_id from 'companies list'", raw)
	}
	return raw, nil
}

// Created presets accepted by audit-report --created (commands/aram.py).
var CreatedPresets = []string{"today", "yesterday", "last_7_days", "last_30_days", "month_to_date", "last_month", "year_to_date", "last_year"}

// SummaryPeriods accepted by audit-report --report-type, raw excluded.
var SummaryPeriods = []string{"day", "week", "month", "span"}

func oneOf(v string, set []string, what string) (string, error) {
	for _, s := range set {
		if v == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid %s %q: use one of %s", what, v, strings.Join(set, ", "))
}

func intIn(v, lo, hi int, what string) (string, error) {
	if v < lo || v > hi {
		return "", fmt.Errorf("invalid %s %d: use %d to %d", what, v, lo, hi)
	}
	return strconv.Itoa(v), nil
}

// Output fields, from the Commander v18.1.8 report code that produces them.
var (
	// commands/msp.py MSPInfoCommand (verbose: node_name, addons as NAME[:SEATS]).
	CompanyFields = []string{"company_id", "company_name", "node", "node_name", "plan", "storage", "addons", "allocated", "active"}
	// commands/msp.py MSPInfoCommand --pricing.
	ProductFields = []string{"category", "name", "code", "price"}
	// commands/msp.py MSPInfoCommand --restriction (Permit Name, Value; json keeps the titles).
	PermitFields = []string{"Permit Name", "Value"}
	// commands/msp.py MSPBillingReportCommand.
	BillingFields = []string{"date", "company", "company_id", "product", "licenses", "rate", "avg_per_day", "initial_licenses", "final_licenses", "max_licenses"}
	// commands/enterprise.py EnterpriseInfoCommand --users (base user_id, email).
	UserFields = []string{"user_id", "email", "name", "status", "transfer_status", "node", "team_count", "role_count", "roles", "2fa_enabled"}
	NodeFields = []string{"node_id", "name", "parent_node", "parent_id", "user_count", "team_count", "role_count"}
	TeamFields = []string{"team_uid", "name", "restricts", "node", "user_count"}
	RoleFields = []string{"role_id", "name", "visible_below", "default_role", "admin", "node", "user_count"}
	// commands/enterprise.py UserReportCommand --last-login.
	LastLoginFields = []string{"email", "name", "status", "transfer_status", "last_login"}
	// commands/security_audit.py SecurityAuditReportCommand (default and --breachwatch).
	SecurityFields    = []string{"email", "name", "sync_pending", "weak", "fair", "medium", "strong", "reused", "unique", "securityScore", "twoFactorChannel", "node"}
	BreachWatchFields = []string{"email", "name", "sync_pending", "at_risk", "passed", "ignored"}
	// commands/aram.py AuditReportCommand raw (RAW_FIELDS + message).
	AuditEventFields = []string{"created", "audit_event_type", "username", "ip_address", "keeper_version", "geo_location", "message"}
	// commands/aram.py AuditReportCommand summary: occurrences, created, audit_event_type.
	AuditSummaryFields = []string{"occurrences", "created", "audit_event_type"}
)

func rows(name string, fields []string, args ...string) Command {
	return Command{name: name, args: append([]string{"--format=json"}, args...), fields: fields, kind: kindRows}
}

// Companies lists every managed company (msp-info --verbose: ids, node
// names and add-on seats).
func Companies() Command { return rows("msp-info", CompanyFields, "--verbose") }

// Company reads one managed company by id.
func Company(id string) (Command, error) {
	v, err := CompanyID(id)
	if err != nil {
		return Command{}, err
	}
	return rows("msp-info", CompanyFields, "--verbose", "--managed-company", v), nil
}

// Products lists MSP products, add-ons and file plans with pricing.
func Products() Command { return rows("msp-info", ProductFields, "--pricing") }

// Permits lists the MSP's permits (allowed products and add-ons, max file plan).
func Permits() Command { return rows("msp-info", PermitFields, "--restriction") }

// Billing is the consumption billing statement for month (YYYY-MM, "" for
// last month), optionally broken down by company and/or date.
func Billing(month string, byCompany, byDate bool) (Command, error) {
	var args []string
	if month != "" {
		if !reMonth.MatchString(month) {
			return Command{}, fmt.Errorf("invalid month %q: use YYYY-MM", month)
		}
		args = append(args, "--month", month)
	}
	if byCompany {
		args = append(args, "--show-company")
	}
	if byDate {
		args = append(args, "--show-date")
	}
	return rows("msp-billing-report", BillingFields, args...), nil
}

// Users lists enterprise users with status, node, team and role counts and
// 2FA. 2fa_enabled is requested explicitly, so its absence means off
// (commands/enterprise.py writes tfa_enabled or ” and JSON drops ”).
func Users() Command {
	return rows("enterprise-info", UserFields, "--users", "--columns", "name,status,transfer_status,node,team_count,role_count,roles,2fa_enabled")
}

// UserPostureFields are the user facts posture needs; roles come as
// {role_id, role_name} objects (--verbose) so membership joins by role id.
var UserPostureFields = []string{"user_id", "email", "name", "status", "transfer_status", "roles", "2fa_enabled"}

// UserPosture lists users with status, 2FA and their roles by id
// (enterprise-info --users --verbose; --verbose only adds ids, "print ids").
func UserPosture() Command {
	c := rows("enterprise-info", UserPostureFields, "--users", "--verbose", "--columns", "name,status,transfer_status,roles,2fa_enabled")
	c.nested = map[string][]string{"roles": {"role_id", "role_name"}}
	return c
}

// Nodes, Teams and Roles list the enterprise structure.
func Nodes() Command { return rows("enterprise-info", NodeFields, "--nodes") }
func Teams() Command { return rows("enterprise-info", TeamFields, "--teams") }
func Roles() Command { return rows("enterprise-info", RoleFields, "--roles") }

// RolePostureFields are the role facts posture needs: the admin flag and
// the require_two_factor enforcement (constants.py REQUIRE_TWO_FACTOR). The
// enforcements object is cut to that one key before anything else sees it.
var RolePostureFields = []string{"role_id", "name", "admin", "enforcements"}

// RolePosture lists roles with their admin flag and 2FA enforcement.
func RolePosture() Command {
	c := rows("enterprise-info", RolePostureFields, "--roles", "--columns", "admin,enforcements")
	c.nested = map[string][]string{"enforcements": {"require_two_factor"}}
	return c
}

// LastLogin is each user's last login within days (user-report --last-login).
func LastLogin(days int) (Command, error) {
	d, err := intIn(days, 1, 3650, "days")
	if err != nil {
		return Command{}, err
	}
	return rows("user-report", LastLoginFields, "--last-login", "--days", d), nil
}

// SecurityAudit is the per-user security audit; breachwatch switches to the
// BreachWatch columns. Only the summary is read: --record-details, --save
// and --attempt-fix are never sent.
func SecurityAudit(breachwatch bool) Command {
	if breachwatch {
		return rows("security-audit-report", BreachWatchFields, "--breachwatch")
	}
	return rows("security-audit-report", SecurityFields)
}

// AuditEvents reads raw audit events, newest first.
func AuditEvents(created string, limit int, username, eventType string) (Command, error) {
	c, err := oneOf(created, CreatedPresets, "--created")
	if err != nil {
		return Command{}, err
	}
	l, err := intIn(limit, 1, 1000, "limit")
	if err != nil {
		return Command{}, err
	}
	args := []string{"--report-type", "raw", "--report-format", "message", "--created", c, "--order", "desc", "--limit", l}
	if username != "" {
		if len(username) > 254 || !reEmail.MatchString(username) {
			return Command{}, fmt.Errorf("invalid --username %q: use the user's email address", username)
		}
		args = append(args, "--username", username)
	}
	if eventType != "" {
		if !reEventType.MatchString(eventType) {
			return Command{}, fmt.Errorf("invalid --event-type %q: use a Keeper audit event type such as login or record_add", eventType)
		}
		args = append(args, "--event-type", eventType)
	}
	return rows("audit-report", AuditEventFields, args...), nil
}

// AuditSummary counts audit events by type per period.
func AuditSummary(period, created string) (Command, error) {
	p, err := oneOf(period, SummaryPeriods, "--period")
	if err != nil {
		return Command{}, err
	}
	c, err := oneOf(created, CreatedPresets, "--created")
	if err != nil {
		return Command{}, err
	}
	return rows("audit-report", AuditSummaryFields, "--report-type", p, "--created", c, "--aggregate", "occurrences", "--columns", "audit_event_type"), nil
}

// SwitchToMC switches the Service Mode session to a managed company.
func SwitchToMC(id string) (Command, error) {
	v, err := CompanyID(id)
	if err != nil {
		return Command{}, err
	}
	return Command{name: "switch-to-mc", args: []string{v}, kind: kindContext}, nil
}

// SwitchToMSP returns the Service Mode session to the MSP.
func SwitchToMSP() Command { return Command{name: "switch-to-msp", kind: kindContext} }
