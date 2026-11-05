// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `webhooks listen`, the receiver Zammad's
// webhooks call. Pinned by this connector's handfixes.json under the
// "webhooks-listener" entry.
//
// Every request is verified, loopback included (a tunnel makes a loopback
// port public): POST only, body capped, the raw bytes HMAC-SHA1 checked in
// constant time before anything parses them. A delivery id seen before is
// dropped. The payload is a doorbell: its IDs are fetched through the API
// and only the fetched record is written. On every start a catch-up read
// records what changed while the listener was down; Zammad gives up after
// five attempts over about two minutes.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"zammad-pp-cli/internal/credkit"

	"github.com/spf13/cobra"

	"zammad-pp-cli/internal/client"
	"zammad-pp-cli/internal/cliutil"
	"zammad-pp-cli/internal/store"
)

const (
	webhookMaxBody    = 1 << 20
	webhookWatermark  = "zammad"
	webhookOverlap    = 5 * time.Minute // clock skew between this machine and Zammad
	webhookSearchPage = 100
)

func newWebhooksListenCmd(flags *rootFlags) *cobra.Command {
	var addr, tlsCert, tlsKey, dbPath string
	cmd := &cobra.Command{
		Use:   "listen",
		Short: "Receive, verify and record Zammad webhooks until stopped",
		Long: `Runs an HTTP listener for the webhooks 'webhooks register' created.
Bound to 127.0.0.1 by default; any other address needs --tls-cert and
--tls-key. Refuses to start without the signature secret (ZAMMAD_WEBHOOK_SECRET
or the one 'register' stored). Every request must carry Zammad's
X-Hub-Signature; the IDs it names are fetched through the API and recorded
for 'events list'. On start it first records what changed while it was down.`,
		Example: `  zammad-cli webhooks listen
  zammad-cli webhooks listen --addr 0.0.0.0:8789 --tls-cert cert.pem --tls-key key.pem`,
		// Long-running, binds a port, reads a key file: never an MCP tool.
		// pp:data-source live
		Annotations: map[string]string{"mcp:hidden": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			// It records events and upserts tickets locally, as articles sync does (Codex nb-r1 [P2]).
			if localDataDisabled() {
				return fmt.Errorf("webhooks listen is disabled while %s=1: it records events and tickets in the local store", noLocalDataEnvVar)
			}
			bind, loopback := classifyListenAddr(addr)
			if (tlsCert == "") != (tlsKey == "") {
				return fmt.Errorf("--tls-cert and --tls-key go together")
			}
			if !loopback && tlsCert == "" {
				return fmt.Errorf("--addr %q is not loopback: it needs --tls-cert and --tls-key; plain HTTP is only allowed on 127.0.0.1", addr)
			}
			secret, _, err := cliutil.LoadWebhookSecret(flags.configPath)
			if err != nil {
				return err
			}
			if secret == "" {
				return fmt.Errorf("no webhook secret: set ZAMMAD_WEBHOOK_SECRET or run 'zammad-cli webhooks register --yes' (it stores one); refusing to accept unsigned requests")
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "webhooks listen on "+bind)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			db, err := store.OpenWithContext(cmd.Context(), defaultNovelDBPath(dbPath))
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			ln, err := net.Listen("tcp", bind)
			if err != nil {
				return err
			}
			l := &webhookListener{secret: []byte(secret), db: db, c: c}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return l.serve(ctx, ln, tlsCert, tlsKey)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8789", "Address to listen on")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate file (required off loopback)")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key file (required off loopback)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// classifyListenAddr mirrors the MCP HTTP server's bind rule: an IP is
// loopback by its value; a name only when every address it resolves to is
// loopback, and then the listener binds that IP.
func classifyListenAddr(addr string) (string, bool) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return addr, false
	}
	if ip := net.ParseIP(host); ip != nil {
		return addr, ip.IsLoopback()
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return addr, false
	}
	var chosen net.IP
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return addr, false
		}
		if chosen == nil || ip.To4() != nil {
			chosen = ip
		}
	}
	return net.JoinHostPort(chosen.String(), port), true
}

type webhookListener struct {
	secret []byte
	db     *store.Store
	c      *client.Client
	// failed is set once a verified event could not be recorded, or the
	// catch-up failed: the next start then reads from this start's time.
	failed atomic.Bool
}

func (l *webhookListener) serve(ctx context.Context, ln net.Listener, tlsCert, tlsKey string) error {
	srv := &http.Server{Handler: l, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute}
	errc := make(chan error, 1)
	scheme := "http"
	if tlsCert != "" {
		scheme = "https"
	}
	go func() {
		if tlsCert != "" {
			errc <- srv.ServeTLS(ln, tlsCert, tlsKey)
		} else {
			errc <- srv.Serve(ln)
		}
	}()
	stderrNow([]byte(fmt.Sprintf("listening on %s://%s/ (Ctrl-C stops)\n", scheme, ln.Addr())))
	if n, err := l.catchUp(ctx); err != nil {
		l.failed.Store(true)
		stderrNow([]byte(fmt.Sprintf("catch-up failed, the next start reads from this start: %v\n", err)))
	} else {
		stderrNow([]byte(fmt.Sprintf("catch-up recorded %d events\n", n)))
	}
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errc:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	// Every verified event was recorded: the next catch-up can start here.
	if !l.failed.Load() {
		if err := l.db.SetVendorWatermark(context.Background(), webhookWatermark, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

func (l *webhookListener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookMaxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !validWebhookSignature(l.secret, body, r.Header.Get("X-Hub-Signature")) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	delivery := r.Header.Get("X-Zammad-Delivery")
	if delivery == "" || len(delivery) > 128 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if seen, err := l.db.VendorDeliverySeen(ctx, delivery); err != nil {
		l.fail(w, http.StatusInternalServerError, "store", err)
		return
	} else if seen {
		w.WriteHeader(http.StatusOK)
		return
	}
	var p struct {
		Event     string          `json:"event"`
		TicketID  json.RawMessage `json:"ticket_id"`
		ArticleID json.RawMessage `json:"article_id"`
	}
	ticketID := ""
	if json.Unmarshal(body, &p) == nil {
		ticketID = payloadID(p.TicketID)
	}
	articleID := payloadID(p.ArticleID)
	if !isWebhookEvent(p.Event) || ticketID == "" || (p.Event == "zammad.article.created" && articleID == "") {
		l.fail(w, http.StatusBadRequest, "payload", errors.New("not a 'webhooks register' payload (event and ticket_id)"))
		return
	}
	ev, err := l.fetchEvent(ctx, p.Event, ticketID, articleID)
	if err != nil {
		l.fail(w, http.StatusBadGateway, "fetch ticket "+ticketID, err)
		return
	}
	ev.Source, ev.DeliveryID = "webhook", delivery
	seq, err := l.record(ctx, ev)
	if errors.Is(err, errEventHoldsSecret) {
		l.fail(w, http.StatusBadRequest, "payload", err)
		return
	}
	if err != nil {
		l.fail(w, http.StatusInternalServerError, "store", err)
		return
	}
	if seq == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	stderrNow([]byte(fmt.Sprintf("recorded %s %s %s (cursor %d)\n", ev.Type, ev.ResourceType, ev.ResourceID, seq)))
	w.WriteHeader(http.StatusOK)
}

// stderrNow writes one whole fixed-template line for a command that runs until
// stopped; the kit checks stderr as it streams.
func stderrNow(p []byte) { _, _ = os.Stderr.Write(p) }

// errEventHoldsSecret refuses a row that repeats a held value.
var errEventHoldsSecret = errors.New("the event repeats a held credential; not recorded")

// record is the one insert into vendor_events, from a delivery or the
// catch-up: the whole row passes the held check first, and a row that
// repeats a held value anywhere (a delivery id is a request header, stored
// as sent) is refused, never stored part withheld (Codex nb-r1).
func (l *webhookListener) record(ctx context.Context, ev store.VendorEvent) (int64, error) {
	n, err := l.db.InsertVendorEvent(ctx, ev)
	if errors.Is(err, credkit.ErrWithheld) { // Hand-fix (handfixes.json credkit-wiring): the kit's SQLite driver refused the statement whole.
		return 0, errEventHoldsSecret
	}
	return n, err
}

// fail answers a verified request that could not be recorded. Zammad
// retries a non-2xx answer; the next start's catch-up covers the rest.
func (l *webhookListener) fail(w http.ResponseWriter, code int, what string, err error) {
	l.failed.Store(true)
	stderrNow([]byte(fmt.Sprintf("event not recorded (%s): %v\n", what, err)))
	w.WriteHeader(code)
}

// payloadID is a numeric id from the payload (Zammad renders "#{ticket.id}"
// as a string), or "" for anything else.
func payloadID(raw json.RawMessage) string {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || len(s) > 18 || strings.Trim(s, "0123456789") != "" {
		return ""
	}
	return s
}

type zammadTicketRef struct {
	ID         int64  `json:"id"`
	Number     string `json:"number"`
	StateID    int64  `json:"state_id"`
	PriorityID int64  `json:"priority_id"`
	GroupID    int64  `json:"group_id"`
	OwnerID    int64  `json:"owner_id"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type zammadArticleRef struct {
	ID        int64  `json:"id"`
	TicketID  int64  `json:"ticket_id"`
	Internal  bool   `json:"internal"`
	CreatedAt string `json:"created_at"`
}

// fetchEvent reads the ticket (and article) the doorbell named and builds
// the event from what the API returned. The event id is the upstream
// identity, so the webhook and the catch-up record one row per change.
func (l *webhookListener) fetchEvent(ctx context.Context, name, ticketID, articleID string) (store.VendorEvent, error) {
	raw, err := l.c.GetNoCache(ctx, "/tickets/"+ticketID, nil)
	if err != nil {
		return store.VendorEvent{}, err
	}
	var t zammadTicketRef
	if err := json.Unmarshal(raw, &t); err != nil || strconv.FormatInt(t.ID, 10) != ticketID {
		return store.VendorEvent{}, fmt.Errorf("ticket %s: unexpected answer", ticketID)
	}
	if err := l.db.UpsertTickets(raw); err != nil {
		stderrNow([]byte(fmt.Sprintf("local ticket copy not refreshed: %v\n", err)))
	}
	if name != "zammad.article.created" {
		return ticketEvent(name, t), nil
	}
	raw, err = l.c.GetNoCache(ctx, "/ticket_articles/"+articleID, nil)
	if err != nil {
		return store.VendorEvent{}, err
	}
	var a zammadArticleRef
	if err := json.Unmarshal(raw, &a); err != nil || a.TicketID != t.ID {
		return store.VendorEvent{}, fmt.Errorf("article %s is not on ticket %s", articleID, ticketID)
	}
	return articleEvent(t, a), nil
}

func ticketData(t zammadTicketRef) map[string]any {
	return map[string]any{"ticket_id": t.ID, "number": t.Number, "state_id": t.StateID, "priority_id": t.PriorityID,
		"group_id": t.GroupID, "owner_id": t.OwnerID, "updated_at": t.UpdatedAt}
}

func ticketEvent(name string, t zammadTicketRef) store.VendorEvent {
	id, at := strconv.FormatInt(t.ID, 10), t.UpdatedAt
	eventID := name + ":" + id + ":" + t.UpdatedAt
	if name == "zammad.ticket.created" {
		eventID, at = name+":"+id, t.CreatedAt
	}
	data, _ := json.Marshal(ticketData(t))
	return store.VendorEvent{EventID: eventID, Type: name, OccurredAt: at, ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano),
		ResourceType: "ticket", ResourceID: id, Verified: true, Data: data}
}

func articleEvent(t zammadTicketRef, a zammadArticleRef) store.VendorEvent {
	d := ticketData(t)
	d["article_id"], d["internal"] = a.ID, a.Internal
	data, _ := json.Marshal(d)
	id := strconv.FormatInt(a.ID, 10)
	return store.VendorEvent{EventID: "zammad.article.created:" + id, Type: "zammad.article.created", OccurredAt: a.CreatedAt,
		ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano), ResourceType: "article", ResourceID: id, Verified: true, Data: data}
}

// catchUp records, as source=sync rows, every ticket and article change
// since the last catch-up (or the last clean stop), less an overlap for
// clock skew; event ids make the overlap free. The first start records
// nothing and starts the clock.
// ponytail: after a crash it re-reads everything since that run's start;
// fine for days, page by updated_at if runs grow to months.
func (l *webhookListener) catchUp(ctx context.Context) (int, error) {
	start := time.Now().UTC().Format(time.RFC3339)
	since, ok, err := l.db.VendorWatermark(ctx, webhookWatermark)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, l.db.SetVendorWatermark(ctx, webhookWatermark, start)
	}
	from, err := time.Parse(time.RFC3339, since)
	if err != nil {
		return 0, fmt.Errorf("stored catch-up time %q: %w", since, err)
	}
	from = from.Add(-webhookOverlap)
	n, prevFirst := 0, ""
	for page := 1; ; page++ {
		raw, err := l.c.GetNoCache(ctx, "/tickets/search", map[string]string{
			"condition[ticket.updated_at][operator]": "after (absolute)",
			"condition[ticket.updated_at][value]":    from.Format(time.RFC3339),
			"sort_by":                                "updated_at", "order_by": "asc", "expand": "true",
			"per_page": strconv.Itoa(webhookSearchPage), "page": strconv.Itoa(page),
		})
		if err != nil {
			return n, err
		}
		var tickets []json.RawMessage
		if err := json.Unmarshal(raw, &tickets); err != nil {
			return n, fmt.Errorf("reading the ticket search: %w", err)
		}
		// A server that ignores page would return page 1 forever.
		if len(tickets) > 0 {
			first := string(tickets[0])
			if first == prevFirst {
				return n, fmt.Errorf("the ticket search repeated a page; stopping")
			}
			prevFirst = first
		}
		for _, tr := range tickets {
			got, err := l.catchUpTicket(ctx, tr, from)
			n += got
			if err != nil {
				return n, err
			}
		}
		if len(tickets) < webhookSearchPage {
			break
		}
	}
	return n, l.db.SetVendorWatermark(ctx, webhookWatermark, start)
}

func (l *webhookListener) catchUpTicket(ctx context.Context, raw json.RawMessage, from time.Time) (int, error) {
	var t zammadTicketRef
	if err := json.Unmarshal(raw, &t); err != nil || t.ID == 0 {
		return 0, fmt.Errorf("reading a searched ticket: unexpected answer")
	}
	_ = l.db.UpsertTickets(raw)
	events := []store.VendorEvent{ticketEvent("zammad.ticket.updated", t)}
	if after(t.CreatedAt, from) {
		events = append(events, ticketEvent("zammad.ticket.created", t))
		if t.CreatedAt == t.UpdatedAt {
			events = events[1:]
		}
	}
	articles, err := l.c.GetNoCache(ctx, "/ticket_articles/by_ticket/"+strconv.FormatInt(t.ID, 10), nil)
	if err != nil {
		return 0, err
	}
	var as []zammadArticleRef
	if err := json.Unmarshal(articles, &as); err != nil {
		return 0, fmt.Errorf("reading ticket %d articles: %w", t.ID, err)
	}
	for _, a := range as {
		if after(a.CreatedAt, from) {
			events = append(events, articleEvent(t, a))
		}
	}
	n := 0
	for _, e := range events {
		e.Source = "sync"
		seq, err := l.record(ctx, e)
		if errors.Is(err, errEventHoldsSecret) { // skip that row, read on; the run is not clean
			l.failed.Store(true)
			stderrNow([]byte(fmt.Sprintf("event not recorded (%s %s): %v\n", e.Type, e.ResourceID, err)))
			continue
		}
		if err != nil {
			return n, err
		}
		if seq > 0 {
			n++
		}
	}
	return n, nil
}

func after(ts string, from time.Time) bool {
	t, err := time.Parse(time.RFC3339Nano, ts)
	return err == nil && t.After(from)
}
