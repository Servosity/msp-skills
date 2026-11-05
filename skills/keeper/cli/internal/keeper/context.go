// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeper

// keeper handfix mc-context-guard: Commander's managed-company context is one
// global per Service Mode process (commands/msp.py current_mc_id), shared by
// every API key, and enterprise commands silently run against whichever
// company it names (cli.py check_if_running_as_mc). A session left in a
// managed company (a crashed run, another tool) would make an MSP-level
// "users list" report that company's users as the MSP's. So every
// enterprise read first returns the session to the MSP, and a --mc read
// switches in, checks Keeper confirmed the switch, reads, and switches back.
//
// keeper handfix mc-context-serialized (Codex r1 class cross-tenant-context-race):
// two reads interleaving their switches (A into 10, B into 20, A reads) would
// label company 20's data as company 10's. So the company travels with each
// request as an argument, this program keeps no tenant state, and
//   - every context transaction (return to the MSP or switch in, read, check,
//     switch back) holds one session lock: a process-wide mutex plus the
//     caller's LockedPoster file, an exclusive OS file lock keyed to the Service
//     Mode origin in one fixed per-user location, so CLI runs under any --home
//     and the MCP server's children never interleave against one instance;
//   - after the read, the context is checked again without changing it:
//     switch-to-mc <id> must answer "Already switched to Managed Company
//     id=<id>" (commands/msp.py SwitchToMcCommand), and an MSP-level read's
//     switch-to-msp must answer "Already MSP". Any other answer means another
//     client moved the shared session during the read: the result is
//     discarded (ErrContextChanged), never shown or stored. The README asks
//     for a Service Mode instance no other tool switches; this check catches
//     a tool that does.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"keeper-pp-cli/internal/cliutil"
)

// ErrContextChanged is returned when the shared session was not in the
// expected context after a read; the read's result is discarded.
var ErrContextChanged = errors.New("another client changed the Service Mode session's managed-company context during the read; the result was discarded (run this CLI against a Service Mode instance no other tool switches)")

// sessionMu orders this process's transactions; it holds no tenant state.
var sessionMu sync.Mutex

// LockedPoster is a Poster that names the file every process of this CLI locks
// for one context transaction against its Service Mode instance.
type LockedPoster interface {
	Poster
	SessionLockFile() string
}

func withSession(p Poster, fn func() error) error {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if lp, ok := p.(LockedPoster); ok {
		return cliutil.WithFileLock(lp.SessionLockFile(), fn)
	}
	return fn()
}

// WithCompany runs read in the MSP context (mc "") or in managed company mc.
// warn receives a note when the session had been left in a managed company.
func WithCompany(ctx context.Context, p Poster, mc string, warn func(string), read func() error) error {
	return withSession(p, func() error { return withCompany(ctx, p, mc, warn, read) })
}

func withCompany(ctx context.Context, p Poster, mc string, warn func(string), read func() error) (err error) {
	if err := toMSP(ctx, p, warn); err != nil {
		return err
	}
	if mc == "" {
		if err := read(); err != nil {
			return err
		}
		return stillMSP(context.WithoutCancel(ctx), p)
	}
	sw, err := SwitchToMC(mc)
	if err != nil {
		return err
	}
	msg, err := Message(ctx, p, sw)
	if err != nil {
		// A retried or half-applied switch may still have moved the session.
		_ = toMSP(context.WithoutCancel(ctx), p, nil)
		return err
	}
	if !strings.Contains(msg, "Switched to MC") {
		// Keeper logs "Switched to MC '<name>'" on success; without it the
		// session state is unknown, so return to the MSP and stop.
		_ = toMSP(context.WithoutCancel(ctx), p, nil)
		return fmt.Errorf("switch-to-mc %s: Keeper did not confirm the switch (%s); nothing was read", mc, quoteShort(msg))
	}
	defer func() {
		// Always return the shared session to the MSP, even when the read
		// failed or the caller's context was cancelled.
		if _, backErr := Message(context.WithoutCancel(ctx), p, SwitchToMSP()); backErr != nil && err == nil {
			err = fmt.Errorf("read managed company %s, but could not return the session to the MSP: %w", mc, backErr)
		}
	}()
	if err := read(); err != nil {
		return err
	}
	return stillIn(context.WithoutCancel(ctx), p, mc)
}

// alreadyInMC is SwitchToMcCommand's refusal while a company is selected.
var alreadyInMC = regexp.MustCompile(`Already switched to Managed Company id=([0-9]+)`)

// stillIn checks, without changing it, that the session is still in mc.
func stillIn(ctx context.Context, p Poster, mc string) error {
	sw, err := SwitchToMC(mc)
	if err != nil {
		return err
	}
	_, err = Message(ctx, p, sw)
	if err == nil {
		// The switch succeeded: the session had left mc (another client
		// returned it to the MSP). The caller switches back.
		return fmt.Errorf("managed company %s: %w", mc, ErrContextChanged)
	}
	m := alreadyInMC.FindStringSubmatch(err.Error())
	if m == nil {
		return fmt.Errorf("managed company %s: could not confirm the session stayed in it: %w", mc, err)
	}
	if m[1] != mc {
		return fmt.Errorf("managed company %s: the session is in managed company %s: %w", mc, m[1], ErrContextChanged)
	}
	return nil
}

// stillMSP checks that an MSP-level read stayed in the MSP context. A
// successful switch-to-msp means another client had switched the session
// into a managed company (it is back in the MSP now).
func stillMSP(ctx context.Context, p Poster) error {
	_, err := Message(ctx, p, SwitchToMSP())
	if err == nil {
		return ErrContextChanged
	}
	if strings.Contains(err.Error(), "Already MSP") {
		return nil
	}
	return fmt.Errorf("could not confirm the session stayed in the MSP context: %w", err)
}

// toMSP returns the session to the MSP; "Already MSP" (commands/msp.py
// SwitchToMspCommand) means it already was.
func toMSP(ctx context.Context, p Poster, warn func(string)) error {
	_, err := Message(ctx, p, SwitchToMSP())
	if err == nil {
		if warn != nil {
			warn("the Service Mode session was in a managed company context; returned it to the MSP before reading")
		}
		return nil
	}
	if strings.Contains(err.Error(), "Already MSP") {
		return nil
	}
	return fmt.Errorf("could not confirm the Service Mode session is in the MSP context: %w", err)
}

// ResetContext returns the session to the MSP and says whether it had to.
func ResetContext(ctx context.Context, p Poster) (bool, error) {
	var moved bool
	err := withSession(p, func() error {
		return toMSP(ctx, p, func(string) { moved = true })
	})
	return moved, err
}
