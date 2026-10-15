package fortios

import (
	"bufio"
	"fmt"
	"sort"
	"strings"
)

// ObjectKey identifies one configuration object in a FortiOS backup.
type ObjectKey struct {
	VDOM  string `json:"vdom,omitempty"`
	Table string `json:"table"`
	Key   string `json:"key,omitempty"`
}

// Config maps each object to its attributes (nested sub-tables are folded
// into dotted attribute names, e.g. "ipv6.ip6-address").
type Config map[ObjectKey]map[string]string

type frame struct {
	edit bool
	name string
}

// ParseConfig parses FortiOS `config / edit / set / next / end` backup
// text. Secrets are redacted at parse time so they never reach output.
// Policy tables also get an "(order)" pseudo-object, because rule order
// decides first-match behavior.
func ParseConfig(text string) (Config, error) {
	out := Config{}
	orders := map[ObjectKey][]string{}
	var stack []frame
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		word, rest, _ := strings.Cut(line, " ")
		switch word {
		case "config":
			stack = append(stack, frame{name: rest})
		case "edit":
			stack = append(stack, frame{edit: true, name: unquote(rest)})
			// Register the object on entry so an edit with no set lines still exists.
			if key, prefix, ok := locate(stack); ok && prefix == "" && key.Key != "" {
				if out[key] == nil {
					out[key] = map[string]string{}
				}
				table := ObjectKey{VDOM: key.VDOM, Table: key.Table}
				orders[table] = append(orders[table], key.Key)
			}
		case "next":
			// next closes the innermost edit; anything else means a damaged file.
			if len(stack) == 0 || !stack[len(stack)-1].edit {
				return nil, fmt.Errorf("unexpected next (no open edit)")
			}
			stack = stack[:len(stack)-1]
		case "end":
			// end closes the innermost config; FortiOS allows an open edit to be
			// closed implicitly by its parent's end.
			if len(stack) > 0 && stack[len(stack)-1].edit {
				stack = stack[:len(stack)-1]
			}
			if len(stack) == 0 || stack[len(stack)-1].edit {
				return nil, fmt.Errorf("unexpected end (no open config)")
			}
			stack = stack[:len(stack)-1]
		case "set":
			attr, value, _ := strings.Cut(rest, " ")
			// Multi-line quoted values (certificates, keys) continue until
			// the closing quote.
			for quoteOpen(value) {
				if !sc.Scan() {
					return nil, fmt.Errorf("unterminated quoted value for %q", attr)
				}
				value += "\n" + sc.Text()
			}
			key, prefix, ok := locate(stack)
			if !ok {
				continue
			}
			if out[key] == nil {
				out[key] = map[string]string{}
			}
			out[key][prefix+attr] = redact(key.Table, prefix, attr, strings.TrimSpace(value))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read backup: %w", err)
	}
	if len(stack) > 0 {
		return nil, fmt.Errorf("backup ends inside an open %q block (truncated file?)", stack[len(stack)-1].name)
	}
	for table, keys := range orders {
		if strings.Contains(table.Table, "policy") && len(keys) > 1 {
			out[ObjectKey{VDOM: table.VDOM, Table: table.Table, Key: "(order)"}] = map[string]string{"order": strings.Join(keys, ",")}
		}
	}
	return out, nil
}

// quoteOpen reports whether s ends inside a double-quoted string, honoring
// backslash escapes inside quotes.
func quoteOpen(s string) bool {
	in, esc := false, false
	for _, r := range s {
		switch {
		case esc:
			esc = false
		case in && r == '\\':
			esc = true
		case r == '"':
			in = !in
		}
	}
	return in
}

func locate(stack []frame) (ObjectKey, string, bool) {
	var k ObjectKey
	rest := stack
	if len(rest) >= 2 && !rest[0].edit && rest[0].name == "vdom" && rest[1].edit {
		k.VDOM, rest = rest[1].name, rest[2:]
	} else if len(rest) >= 1 && !rest[0].edit && rest[0].name == "global" {
		k.VDOM, rest = "global", rest[1:]
	}
	if len(rest) == 0 || rest[0].edit {
		return k, "", false
	}
	k.Table, rest = rest[0].name, rest[1:]
	if len(rest) > 0 && rest[0].edit {
		k.Key, rest = rest[0].name, rest[1:]
	}
	var prefix strings.Builder
	for _, f := range rest {
		prefix.WriteString(f.name)
		prefix.WriteString(".")
	}
	return k, prefix.String(), true
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

var secretAttrs = map[string]bool{
	"password": true, "passwd": true, "psksecret": true, "private-key": true, "secret": true,
	"key": true, "api-key": true, "passphrase": true, "auth-pwd": true, "server-key": true,
}

// secretTableAttrs are attributes that are secrets only in a given table:
// an SNMP v1/v2c community's name is the community string itself, and an
// automation webhook URI (Slack, Teams) is a bearer capability, and its
// body can carry an integration key (PagerDuty routing_key).
var secretTableAttrs = map[string]map[string]bool{
	"system snmp community":    {"name": true},
	"system automation-action": {"uri": true, "http-body": true},
}

// redact hides secret values. Every attribute under an http-headers table is
// hidden: webhook header values carry bearer tokens and API keys.
func redact(table, prefix, attr, value string) string {
	if strings.HasPrefix(strings.TrimLeft(value, `"`), "ENC ") || strings.Contains(value, "-----BEGIN") || secretAttrs[attr] ||
		strings.HasPrefix(prefix, "http-headers.") ||
		strings.HasSuffix(attr, "-pwd") || strings.HasSuffix(attr, "password") || secretTableAttrs[table][attr] {
		return "<redacted>"
	}
	return value
}

// Change is one added, removed or modified object.
type Change struct {
	ObjectKey
	Kind       string               `json:"kind"`
	Attributes map[string][2]string `json:"attributes,omitempty"`
}

var securityTables = map[string]int{
	"system admin": 0, "system api-user": 1, "firewall policy": 2, "vpn ssl settings": 3,
	"firewall local-in-policy": 4, "system interface": 5, "user local": 6, "firewall vip": 7,
}

// Diff compares two parsed configs. Attribute pairs are [old, new].
// Security-relevant tables sort first.
func Diff(old, cur Config) []Change {
	out := []Change{}
	for k, attrs := range cur {
		prev, ok := old[k]
		if !ok {
			out = append(out, Change{ObjectKey: k, Kind: "added"})
			continue
		}
		changed := map[string][2]string{}
		for a, v := range attrs {
			if prev[a] != v {
				changed[a] = [2]string{prev[a], v}
			}
		}
		for a, v := range prev {
			if _, ok := attrs[a]; !ok {
				changed[a] = [2]string{v, ""}
			}
		}
		if len(changed) > 0 {
			out = append(out, Change{ObjectKey: k, Kind: "modified", Attributes: changed})
		}
	}
	for k := range old {
		if _, ok := cur[k]; !ok {
			out = append(out, Change{ObjectKey: k, Kind: "removed"})
		}
	}
	rank := func(t string) int {
		if r, ok := securityTables[t]; ok {
			return r
		}
		return len(securityTables)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if rank(a.Table) != rank(b.Table) {
			return rank(a.Table) < rank(b.Table)
		}
		if a.VDOM != b.VDOM {
			return a.VDOM < b.VDOM
		}
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		return a.Key < b.Key
	})
	return out
}
