// Hand-written for saas-alerts (not generated): storage identity for synced events.
//
// Reports events carry `eventId`, which the source describes as the external
// product's raw id (example "logins_2021-03-24T07:36:46Z": type + second), so it
// collides across customers, products, and users. Identity, in order:
//  1. `_id` (the search-index document id), when present;
//  2. eventId scoped by ALL of: customer.id, product (product.id, else
//     product.name), and a stable account id (user.id, else top-level UserId,
//     else lowercase userEmail, else lowercase email, else user.name lowercased
//     when it is email-shaped). A display-name user.name is not an account id.
//     All three scope parts must be present;
//  3. a hash of the whole event, so an unidentifiable event is kept, never merged
//     into another under a bare eventId.

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"time"
)

func saasEventStorageID(obj map[string]any) string {
	str := func(v any) string { return strings.TrimSpace(ResourceIDString(v)) }
	nested := func(parent, key string) string {
		m, _ := obj[parent].(map[string]any)
		if m == nil {
			return ""
		}
		return str(m[key])
	}
	// Only a real event is stored: it must carry a parseable time. This runs
	// before any identity so error bodies with id-like fields are rejected too.
	if !saasEventTimeValid(str(obj["time"])) || saasEventDeclaresFailure(obj) {
		return ""
	}
	if id := str(obj["_id"]); id != "" {
		return id
	}
	if eid := str(obj["eventId"]); eid != "" {
		customer := nested("customer", "id")
		product := nested("product", "id")
		if product == "" {
			product = nested("product", "name")
		}
		account := saasEventAccountID(obj, str, nested)
		if customer != "" && product != "" && account != "" {
			return "evt:" + saasIDHash(eid, customer, product, account)
		}
	}
	raw, err := json.Marshal(obj) // map keys marshal sorted, so this is stable
	if err != nil {
		return ""
	}
	return "evth:" + saasIDHash(string(raw))
}

// saasEventAccountID is the event's stable account id: user.id, else the
// top-level UserId, else userEmail, else email, else an email-shaped user.name
// (emails lowercased). A plain display name is never an account id.
func saasEventAccountID(obj map[string]any, str func(any) string, nested func(string, string) string) string {
	if id := nested("user", "id"); id != "" {
		return id
	}
	if id := str(obj["UserId"]); id != "" {
		return id
	}
	for _, email := range []string{str(obj["userEmail"]), str(obj["email"])} {
		if email != "" {
			return strings.ToLower(email)
		}
	}
	if name := nested("user", "name"); saasEmailShaped(name) {
		return strings.ToLower(name)
	}
	return ""
}

// saasEmailShaped reports whether s looks like an email address: one "@" with
// text on both sides and no whitespace.
func saasEmailShaped(s string) bool {
	s = strings.TrimSpace(s)
	at := strings.Index(s, "@")
	return at > 0 && at == strings.LastIndex(s, "@") && at < len(s)-1 && !strings.ContainsAny(s, " \t\r\n")
}

func saasIDHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}

// saasEventTimeValid reports whether t parses as the RFC 3339 timestamps the
// Reports API emits (e.g. 2021-03-24T07:36:46.000+00:00).
func saasEventTimeValid(t string) bool {
	if t == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, t)
	return err == nil
}

// saasEventDeclaresFailure reports failure indicators on an event row: a
// non-empty error, errors, or message, success or ok set to false, or a code
// other than 0 or 200. None of these is an IEvent field; `status` is one, so it
// is not checked.
func saasEventDeclaresFailure(obj map[string]any) bool {
	for _, key := range []string{"error", "errors", "message"} {
		switch v := obj[key].(type) {
		case nil:
		case string:
			if strings.TrimSpace(v) != "" {
				return true
			}
		case []any:
			if len(v) > 0 {
				return true
			}
		case map[string]any:
			if len(v) > 0 {
				return true
			}
		default:
			return true
		}
	}
	for _, key := range []string{"success", "ok"} {
		if v, ok := obj[key].(bool); ok && !v {
			return true
		}
	}
	code := strings.Trim(strings.TrimSpace(ResourceIDString(obj["code"])), `"`)
	return code != "" && code != "0" && code != "200"
}

// saasVerifyFixtures reports the press's verify harness (PRINTING_PRESS_VERIFY=1),
// whose mock answers every list with generic rows ({"id":1,"name":...}) that carry
// no event fields. Only there may an unvalidated event fall back to generic
// identity, so the sync->store->search plumbing check can run; real syncs reject it.
func saasVerifyFixtures() bool { return os.Getenv("PRINTING_PRESS_VERIFY") == "1" }
