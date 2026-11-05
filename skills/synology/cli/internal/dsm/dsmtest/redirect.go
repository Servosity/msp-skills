// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsmtest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// BackupLocation is the same-origin redirect target a hostile NAS (or
// anything on the path) answers an authenticated read with: DSM's backup
// start, session id included. Following it starts a backup through a read.
const BackupLocation = "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=backup&version=1&task_ids=%5B5%5D&_sid=SID1"

// RedirectingNAS is a DSM stand-in that answers the login probe and the
// login (sid SID1, SynoToken TOK1), records every request it receives, and
// answers each other request through Answer.
type RedirectingNAS struct {
	*httptest.Server
	mu  sync.Mutex
	log []string
}

// NewRedirectingNAS starts a stand-in whose non-login answers come from
// answer (nil answer: a 302 to BackupLocation for every request that is not
// the backup itself, which answers success).
func NewRedirectingNAS(t *testing.T, answer http.HandlerFunc) *RedirectingNAS {
	t.Helper()
	n := &RedirectingNAS{}
	if answer == nil {
		answer = func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("method") == "backup" {
				_, _ = io.WriteString(w, `{"success":true,"data":{}}`)
				return
			}
			http.Redirect(w, r, BackupLocation, http.StatusFound)
		}
	}
	n.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n.mu.Lock()
		n.log = append(n.log, r.Method+" "+r.URL.RequestURI()+" "+string(body))
		n.mu.Unlock()
		if ServeAPIInfo(w, r) {
			return
		}
		if r.Method == http.MethodPost && strings.Contains(string(body), "method=login") {
			_, _ = io.WriteString(w, `{"success":true,"data":{"sid":"SID1","synotoken":"TOK1"}}`)
			return
		}
		answer(w, r)
	}))
	t.Cleanup(n.Server.Close)
	return n
}

// Requests lists every request received: "METHOD /request-uri body".
func (n *RedirectingNAS) Requests() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.log...)
}

// Count reports how many requests contained sub.
func (n *RedirectingNAS) Count(sub string) int {
	c := 0
	for _, r := range n.Requests() {
		if strings.Contains(r, sub) {
			c++
		}
	}
	return c
}
