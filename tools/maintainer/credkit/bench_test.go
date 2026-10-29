// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The cost targets (contract 3's budget is reached only by adversarial
// input): 150 KB of JSON and a 7 KB process environment are each checked in
// under 200 ms with 20 held credentials, and a credential with non-ASCII
// characters does not exhaust the budget on ordinary output.
//
//	go test -run '^$' -bench . -benchtime 5x

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// benchCreds is 20 credentials of the shapes connectors hold.
func benchCreds() []string {
	r := rand.New(rand.NewPCG(1, 2))
	pick := func(alpha string, n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = alpha[r.IntN(len(alpha))]
		}
		return string(b)
	}
	const (
		hexA = "0123456789abcdef"
		alnA = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
		b64A = alnA + "+/"
	)
	return []string{
		pick(hexA, 40), pick(hexA, 32), pick(b64A, 43) + "=",
		pick(hexA, 8) + "-" + pick(hexA, 4) + "-" + pick(hexA, 4) + "-" + pick(hexA, 4) + "-" + pick(hexA, 12),
		"eyJ" + pick(alnA, 33) + "." + pick(alnA, 60) + "." + pick(alnA, 43),
		"Sk3tch-Pa55word-QX7", "AKIA" + pick("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", 16), pick(b64A, 40),
		"xoxb-" + pick("0123456789", 12) + "-" + pick(alnA, 24), "ghp_" + pick(alnA, 36),
		pick(alnA, 64), pick(alnA, 24), pick(hexA, 64), "sk_live_" + pick(alnA, 24),
		pick(alnA, 16), pick(alnA, 20), "glpat-" + pick(alnA, 20), pick("0123456789", 10),
		pick(alnA, 32), pick(alnA, 48),
	}
}

// benchJSON is a vendor list response of about n bytes.
func benchJSON(n int) []byte {
	r := rand.New(rand.NewPCG(3, 4))
	words := strings.Fields("printer offline network share mailbox quota exceeded vpn tunnel down backup failed " +
		"license renewal laptop replacement password reset request user onboarding firewall rule update " +
		"server reboot scheduled disk space warning antivirus alert phishing report calendar sync issue")
	var b strings.Builder
	b.WriteString(`{"tickets":[`)
	for i := 0; b.Len() < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		title := strings.Join([]string{words[r.IntN(len(words))], words[r.IntN(len(words))], words[r.IntN(len(words))]}, " ")
		var body []string
		for range 8 + r.IntN(12) {
			body = append(body, words[r.IntN(len(words))])
		}
		fmt.Fprintf(&b, `{"id":%d,"number":"%d","title":%q,"group_id":%d,"state":"%s","priority_id":%d,`+
			`"customer":{"id":%d,"email":"user%d@example.com","firstname":"Jane","lastname":"Doe %d"},`+
			`"created_at":"2026-10-%02dT%02d:%02d:%02d.%03dZ","updated_at":"2026-10-%02dT%02d:%02d:%02dZ",`+
			`"article_count":%d,"tags":["%s","%s"],"note":%q}`,
			10000+i, 85000+i, title, 1+r.IntN(9), []string{"open", "new", "pending reminder", "closed"}[r.IntN(4)], 1+r.IntN(3),
			200+r.IntN(5000), r.IntN(900), r.IntN(99),
			1+r.IntN(28), r.IntN(24), r.IntN(60), r.IntN(60), r.IntN(1000), 1+r.IntN(28), r.IntN(24), r.IntN(60), r.IntN(60),
			r.IntN(40), words[r.IntN(len(words))], words[r.IntN(len(words))], strings.Join(body, " "))
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}

// benchEnv is a process environment of about 7 KB, as a child's argv and
// environment unit.
func benchEnv() []byte {
	lines := []string{
		"/usr/local/bin/zammad-cli", "tickets", "list", "--json",
		"PATH=/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/srv/jane/.cargo/bin:/srv/jane/go/bin:/srv/jane/.local/bin",
		"HOME=/srv/jane", "SHELL=/bin/zsh", "USER=jane", "LOGNAME=jane", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8",
		"TERM=xterm-256color", "TERM_PROGRAM=iTerm.app", "TERM_PROGRAM_VERSION=3.5.4", "COLORTERM=truecolor",
		"TMPDIR=/var/folders/7k/q1x2y3z4abc5d6e7f8g9h0000gn/T/", "XPC_FLAGS=0x0", "XPC_SERVICE_NAME=0",
		"__CF_USER_TEXT_ENCODING=0x1F5:0x0:0x0", "SSH_AUTH_SOCK=/private/tmp/com.apple.launchd.Ab12Cd34Ef/Listeners",
		"PWD=/srv/jane/work/msp", "OLDPWD=/srv/jane", "SHLVL=2", "EDITOR=vim", "PAGER=less", "LESS=-R",
		"GOPATH=/srv/jane/go", "GOROOT=/opt/homebrew/Cellar/go/1.26.3/libexec", "GOFLAGS=-mod=mod",
		"HOMEBREW_PREFIX=/opt/homebrew", "HOMEBREW_CELLAR=/opt/homebrew/Cellar", "HOMEBREW_REPOSITORY=/opt/homebrew",
		"MANPATH=/opt/homebrew/share/man::", "INFOPATH=/opt/homebrew/share/info:",
		"ZAMMAD_URL=https://helpdesk.example.com", "ZAMMAD_TIMEOUT=30s", "NODE_ENV=production",
		"ITERM_SESSION_ID=w0t0p0:3F2504E0-4F89-11D3-9A0C-0305E82C3301", "ITERM_PROFILE=Default",
		"LSCOLORS=Gxfxcxdxbxegedabagacad", "LS_COLORS=di=1;36:ln=35:so=32:pi=33:ex=31:bd=34;46:cd=34;43:su=30;41:sg=30;46:tw=30;42:ow=30;43",
		"_=/usr/local/bin/zammad-cli", "COMMAND_MODE=unix2003", "SECURITYSESSIONID=186a5",
	}
	for i := 0; len(strings.Join(lines, "\n")) < 7<<10; i++ {
		lines = append(lines, fmt.Sprintf("APP_SETTING_%03d=value-%d-%s", i, i*7919, strings.Repeat("abcdef", 1+i%4)))
	}
	return []byte(strings.Join(lines, "\n"))
}

func benchHold(b *testing.B, extra ...string) {
	resetForTest(Declarations{})
	Hold("bench", append(benchCreds(), extra...)...)
	currentMatcher()
}

func benchCheck(b *testing.B, unit []byte, extra ...string) {
	benchHold(b, extra...)
	m, _ := currentMatcher()
	// The verdict is reported, not asserted: TestCostOrdinaryOutputIsClean
	// asserts it, and a benchmark run still times an exhausted check.
	v := m.check(unit)
	b.SetBytes(int64(len(unit)))
	b.ResetTimer()
	cpu := cpuTime()
	for range b.N {
		m.check(unit)
	}
	cpu = cpuTime() - cpu
	b.ReportMetric(float64(v), "verdict") // 0 clean, 1 leak, 2 exhausted
	b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/check")
	b.ReportMetric(cpu/float64(b.N), "cpu-ms/check") // garbage collection included
}

// nonASCIICred is a credential with non-ASCII characters.
const nonASCIICred = "pässwörd-ünïcode-Größe"

func BenchmarkJSON150K(b *testing.B)         { benchCheck(b, benchJSON(150<<10)) }
func BenchmarkEnv7K(b *testing.B)            { benchCheck(b, benchEnv()) }
func BenchmarkJSON150KNonASCII(b *testing.B) { benchCheck(b, benchJSON(150<<10), nonASCIICred) }
func BenchmarkEnv7KNonASCII(b *testing.B)    { benchCheck(b, benchEnv(), nonASCIICred) }
