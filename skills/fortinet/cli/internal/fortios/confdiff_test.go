package fortios

import (
	"strings"
	"testing"
)

const oldConf = `#config-version=FGT60F-7.04-FW-build2573:opmode=0:vdom=0:user=admin
config system global
    set hostname "FGT-HQ"
    set admintimeout 5
end
config system admin
    edit "admin"
        set accprofile "super_admin"
        set password ENC SH2abcdef
    next
end
config firewall policy
    edit 1
        set name "lan-out"
        set srcaddr "all"
    next
    edit 2
        set name "temp"
    next
end
config vpn certificate local
    edit "vpn-cert"
        set private-key "-----BEGIN ENCRYPTED TEST KEY-----
MIIabc
-----END ENCRYPTED TEST KEY-----"
    next
end
`

const newConf = `config system global
    set hostname "FGT-HQ"
    set admintimeout 30
end
config system admin
    edit "admin"
        set accprofile "super_admin"
        set password ENC SH2zzzzzz
    next
    edit "contractor"
        set accprofile "prof_admin"
    next
end
config system interface
    edit "port1"
        set allowaccess ping https
        config ipv6
            set ip6-address ::/0
        end
    next
end
config firewall policy
    edit 1
        set name "lan-out"
        set srcaddr "all"
    next
end
`

func TestParseConfig(t *testing.T) {
	c := mustParse(t, oldConf)
	if got := c[ObjectKey{Table: "system global"}]["hostname"]; got != `"FGT-HQ"` {
		t.Fatalf("hostname = %q", got)
	}
	if got := c[ObjectKey{Table: "system admin", Key: "admin"}]["password"]; got != "<redacted>" {
		t.Fatalf("password not redacted: %q", got)
	}
	if got := c[ObjectKey{Table: "vpn certificate local", Key: "vpn-cert"}]["private-key"]; got != "<redacted>" {
		t.Fatalf("private key not redacted: %q", got)
	}
	n := mustParse(t, newConf)
	if got := n[ObjectKey{Table: "system interface", Key: "port1"}]["ipv6.ip6-address"]; got != "::/0" {
		t.Fatalf("nested attr = %q", got)
	}
}

func TestParseConfigMultiVDOM(t *testing.T) {
	conf := `config global
config system admin
edit "admin"
set accprofile "super_admin"
next
end
end
config vdom
edit root
config firewall policy
edit 7
set name "r"
next
end
next
edit guest
config firewall policy
edit 7
set name "g"
next
end
next
end
`
	c := mustParse(t, conf)
	if c[ObjectKey{VDOM: "global", Table: "system admin", Key: "admin"}]["accprofile"] == "" {
		t.Fatalf("global admin missing: %v", c)
	}
	if c[ObjectKey{VDOM: "root", Table: "firewall policy", Key: "7"}]["name"] != `"r"` ||
		c[ObjectKey{VDOM: "guest", Table: "firewall policy", Key: "7"}]["name"] != `"g"` {
		t.Fatalf("per-vdom policies wrong: %v", c)
	}
}

func TestDiff(t *testing.T) {
	changes := Diff(mustParse(t, oldConf), mustParse(t, newConf))
	kinds := map[ObjectKey]string{}
	for _, ch := range changes {
		kinds[ch.ObjectKey] = ch.Kind
	}
	want := map[ObjectKey]string{
		{Table: "system admin", Key: "contractor"}:        "added",
		{Table: "firewall policy", Key: "2"}:              "removed",
		{Table: "system global"}:                          "modified",
		{Table: "system interface", Key: "port1"}:         "added",
		{Table: "vpn certificate local", Key: "vpn-cert"}: "removed",
	}
	for k, v := range want {
		if kinds[k] != v {
			t.Errorf("%v = %q want %q", k, kinds[k], v)
		}
	}
	if _, ok := kinds[ObjectKey{Table: "system admin", Key: "admin"}]; ok {
		t.Error("admin flagged changed although only a redacted secret differed")
	}
	if len(changes) == 0 || changes[0].Table != "system admin" {
		t.Errorf("security tables should sort first: %+v", changes)
	}
	if got := Diff(mustParse(t, newConf), mustParse(t, newConf)); len(got) != 0 {
		t.Errorf("identical configs diff = %+v", got)
	}
}

func mustParse(t *testing.T, text string) Config {
	t.Helper()
	c, err := ParseConfig(text)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	return c
}

func TestParseConfigEscapesEmptyObjectsAndOrder(t *testing.T) {
	const tail = `config system admin
    edit "admin"
        set password ENC SH2abc
    next
end
`
	conf := `config firewall policy
    edit 1
        set comments "a\"b"
        set name "x"
    next
    edit 2
    next
end
` + tail
	reordered := `config firewall policy
    edit 2
    next
    edit 1
        set comments "a\"b"
        set name "x"
    next
end
` + tail
	c := mustParse(t, conf)
	// An escaped quote must not swallow the following lines (or the ENC secret).
	if got := c[ObjectKey{Table: "firewall policy", Key: "1"}]["name"]; got != `"x"` {
		t.Fatalf("escaped quote broke parsing: name=%q (%v)", got, c)
	}
	if got := c[ObjectKey{Table: "system admin", Key: "admin"}]["password"]; got != "<redacted>" {
		t.Fatalf("password leaked: %q", got)
	}
	if _, ok := c[ObjectKey{Table: "firewall policy", Key: "2"}]; !ok {
		t.Fatal("empty edit object missing")
	}
	// Reordering policies is a change (first match wins).
	changes := Diff(c, mustParse(t, reordered))
	if len(changes) != 1 || changes[0].Key != "(order)" || changes[0].Kind != "modified" {
		t.Fatalf("reorder changes = %+v", changes)
	}
	if _, err := ParseConfig("config system global\n    set post-login-banner \"open\n"); err == nil {
		t.Fatal("unterminated quote accepted")
	}
	if strings.Contains(conf, "\\\\") {
		t.Fatal("fixture should use a single-backslash escape")
	}
}

func TestParseConfigRejectsBrokenStructureAndRedactsCommunity(t *testing.T) {
	for _, bad := range []string{"config system global\n    set a 1\n", "next\n", "end\n", "config firewall policy\n    edit 1\n    next\n    next\nend\n"} {
		if _, err := ParseConfig(bad); err == nil {
			t.Errorf("accepted broken config %q", bad)
		}
	}
	c := mustParse(t, "config system snmp community\n    edit 1\n        set name \"s3cret-community\"\n    next\nend\n")
	if got := c[ObjectKey{Table: "system snmp community", Key: "1"}]["name"]; got != "<redacted>" {
		t.Fatalf("SNMP community leaked: %q", got)
	}
}

func TestRedactQuotedENC(t *testing.T) {
	if got := redact("vpn ipsec phase1-interface", "", "ppk-secret", `"ENC QUOTEDPPK"`); got != "<redacted>" {
		t.Errorf("quoted ENC value = %q, want <redacted>", got)
	}
	if got := redact("firewall address", "", "comment", `"ENCyclopedia"`); got == "<redacted>" {
		t.Error("a non-secret value starting with ENC was redacted")
	}
}

func TestParseConfigRedactsWebhookCredentials(t *testing.T) {
	c := mustParse(t, "config system automation-action\n    edit \"notify\"\n        set action-type webhook\n        set uri \"hooks.example.com/SENTINEL-URI\"\n        set http-body \"{\\\"routing_key\\\":\\\"SENTINEL-BODY\\\"}\"\n        config http-headers\n            edit 1\n                set key \"Authorization\"\n                set value \"Bearer SENTINEL-HDR\"\n            next\n        end\n    next\nend\n")
	o := c[ObjectKey{Table: "system automation-action", Key: "notify"}]
	if o["uri"] != "<redacted>" || o["http-body"] != "<redacted>" || o["http-headers.1.value"] != "<redacted>" || o["action-type"] != "webhook" {
		t.Fatalf("webhook credentials not redacted: %v", o)
	}
}
