// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// Codex review round 1 (aws-billing 4.34.2 reprint): one test per P1 class,
// each run against the code Codex reviewed and seen failing there
// (seen/r1-<class>-seen-failing.log in the run directory):
//
//   - credential-sent-to-another-host: the container credential provider's
//     own HTTP client skipped the endpoint pin;
//   - holder-incomplete: SDK credential sources the startup holder did not
//     read (the AWS_ACCESS_KEY / AWS_SECRET_KEY aliases, colon-delimited,
//     quoted and multi-line shared-file values, a profile's token file, the
//     login cache directory and its identityToken);
//   - credential-handed-to-another-program: a helper's environment, argv,
//     stdin and output path;
//   - secret-in-output-before-holder: a credential_process's stderr;
//   - cache-answer-not-the-request: the local cost cache answered a request
//     with another granularity's, another interval's or another account's
//     rows, and the inventory with another account's;
//   - requested-dimension-never-fetched: bill --group-by usage-type/region;
//   - failure-read-as-an-empty-result: ask, a prior period, a stored
//     incomplete inventory scan, a denied account list;
//   - delegated-command-drops-caller-scope: ask's transfer answer;
//   - wrong-field-from-a-documented-response: a copied snapshot's volume ID;
//   - wrong-resource-region: S3 buckets stamped with the scan region.

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const (
	r1AliasKey     = "AKIA" + "BBALIASKEY5N2M7Q"
	r1AliasSecret  = "bbAliasSecretKey/8hJ3kL6mN9pQ2rS5"
	r1ColonSecret  = "bbColonSecret+4vW7xY0zA3bC6dE9"
	r1QuotedSecret = "bbQuotedSecret/2fG5hJ8kL1mN4pQ7"
	r1ContHead     = "bbCtHeadQ6qR9sT2uV5"
	r1ContTail     = "bbCtTailM3wX8yZ1nB4"
	r1WebIDToken   = "bbPrfWebId" + "K7a3c9e1f5b"
	r1LoginID      = "bbLgnIdTok3e8a1c6f9b2d4"
	r1LoginDirTok  = "bbLgnDirRfr6d2f8b4a1c9e7"
	r1ProcSecret   = "bbProcStderrSecret/5kM8nP1qR4sT7uV"
	r1ProcParse    = "bbProcParseSecret/3hK6mP9rT2wY5zB"
)

// r1JSONHas reports a JSON member key with string value v in out, however
// the JSON is spaced.
func r1JSONHas(out, key, v string) bool {
	return regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*"` + regexp.QuoteMeta(v) + `"`).MatchString(out)
}

// r1Total reads total_usd from a bill --json answer.
func r1Total(t *testing.T, out string) (float64, bool) {
	t.Helper()
	var v struct {
		Total *float64 `json:"total_usd"`
	}
	start := strings.Index(out, "{")
	if start < 0 || json.NewDecoder(strings.NewReader(out[start:])).Decode(&v) != nil || v.Total == nil {
		return 0, false
	}
	return *v.Total, true
}

func TestR1ContainerCredentialEndpointIsPinned(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcContainer).with("AWS_CONTAINER_CREDENTIALS_FULL_URI=https://collector.example/credentials")
	out := e.run(t, "bill", "--data-source", "live")
	e.trip.mu.Lock()
	hits := append([]string(nil), e.trip.hits...)
	e.trip.mu.Unlock()
	if len(hits) > 0 {
		t.Errorf("the container credential provider dialed a host outside AWS (it would have sent AWS_CONTAINER_AUTHORIZATION_TOKEN there): %v", hits)
	}
	assertNoEcho(t, "bill output", out)
}

func TestR1HolderReadsEverySDKCredentialSource(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcProfileFlag)
	creds := filepath.Join(e.home, ".aws", "credentials")
	e.input(t, creds, "[default]\naws_access_key_id = "+bbDefKey+"\naws_secret_access_key = "+bbDefSecret+"\n\n"+
		"[bbfile]\naws_access_key_id = "+bbFileKey+"\naws_secret_access_key = "+bbFileSecret+"\n\n"+
		"[bbcolon]\naws_access_key_id: AKIA"+"BBCOLONKEY1A2B3C\naws_secret_access_key: "+r1ColonSecret+"\n\n"+
		"[bbquoted]\naws_access_key_id = AKIA"+"BBQUOTEDKEY4D5E6\naws_secret_access_key = \""+r1QuotedSecret+"\"\n\n"+
		"[bbcont]\naws_access_key_id = AKIA"+"BBCONTKEY7F8G9H0\naws_secret_access_key = bbContSecretKey/1aB4cD7eF0\n"+
		"aws_session_token = "+r1ContHead+"\n  "+r1ContTail+"\n")
	token := filepath.Join(e.home, ".aws", "bbwebid.token")
	e.input(t, token, r1WebIDToken+"\n")
	cfg, err := os.ReadFile(filepath.Join(e.home, ".aws", "config"))
	if err != nil {
		t.Fatal(err)
	}
	e.input(t, filepath.Join(e.home, ".aws", "config"), string(cfg)+"\n[profile bbwebid]\nrole_arn = arn:aws:iam::111111111111:role/bbweb\n"+
		"web_identity_token_file = "+token+"\nregion = us-east-1\n")
	login, _ := json.Marshal(map[string]any{"accessToken": map[string]any{"accessKeyId": "ASIA" + "BBLOGINKEY000001"},
		"identityToken": r1LoginID, "clientId": "arn:aws:signin:::devtools/same-device", "tokenType": "aws_sigv4"})
	e.input(t, filepath.Join(e.home, ".aws", "login", "cache", "bblogin.json"), string(login))
	dir := filepath.Join(e.home, "logincache")
	other, _ := json.Marshal(map[string]any{"refreshToken": r1LoginDirTok})
	e.input(t, filepath.Join(dir, "bbother.json"), string(other))
	e = e.with("AWS_ACCESS_KEY="+r1AliasKey, "AWS_SECRET_KEY="+r1AliasSecret, "AWS_LOGIN_CACHE_DIRECTORY="+dir)

	const control = "bbControlQueryValue7x"
	if out, code := e.runCode(t, "", bbCLI, "recall", control, "--agent", "--no-learn"); !strings.Contains(out, control) {
		t.Fatalf("recall does not repeat its query; the test cannot see a held value (%s)", quietReason(out, code))
	}
	for name, s := range map[string]string{
		"AWS_ACCESS_KEY": r1AliasKey, "AWS_SECRET_KEY": r1AliasSecret, "a colon-delimited shared-file value": r1ColonSecret,
		"a quoted shared-file value": r1QuotedSecret, "a multi-line session token's first line": r1ContHead,
		"a multi-line session token's continuation": r1ContTail, "a profile's web_identity_token_file": r1WebIDToken,
		"the login cache's identityToken": r1LoginID, "a file in AWS_LOGIN_CACHE_DIRECTORY": r1LoginDirTok,
	} {
		if out := e.run(t, "recall", s, "--agent", "--no-learn"); strings.Contains(out, s) {
			t.Errorf("%s is not held: recall printed it:\n%.600s", name, around(out, s))
		}
	}
}

func TestR1HelperProcessesGetNoCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the slack-pp-cli stand-in is a shell script")
	}
	t.Parallel()
	e := newEnv(t, srcEnv).with("AWS_ACCESS_KEY="+r1AliasKey, "AWS_SECRET_KEY="+r1AliasSecret, "BB_OTHER_VAR=x"+bbEnvSecret)
	e.aws.set(func(f *fakeAWS) { f.quiet = true })
	e.api.quiet = true
	args := []string{"report", "--period", "this-month", "--data-source", "live", "--post-slack", "--yes"}
	e.run(t, append(args, "--slack-channel", "C123")...)
	runs := e.slackRuns(t)
	if len(runs) == 0 {
		t.Fatal("report --post-slack --yes started no slack-pp-cli; the test did not run")
	}
	for id, r := range runs {
		for _, s := range []string{r1AliasKey, r1AliasSecret, bbEnvSecret} {
			if strings.Contains(r["env"], s) {
				t.Errorf("slack run %s started with a credential in its environment: %s", id, around(r["env"], s))
			}
		}
	}
	// A held value given as the channel never reaches the helper's argv or stdin.
	e.run(t, append(args, "--slack-channel", bbEnvKey)...)
	for id, r := range e.slackRuns(t) {
		if strings.Contains(r["argv"], bbEnvKey) || strings.Contains(r["stdin"], bbEnvKey) {
			t.Errorf("slack run %s received a held credential as the channel: %s", id, around(r["argv"]+r["stdin"], bbEnvKey))
		}
	}
}

func TestR1CredentialProcessOutputWithheld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the credential_process stand-in is a shell script")
	}
	t.Parallel()
	e := newEnv(t, srcProfileFlag)
	good := filepath.Join(e.home, "cred.sh")
	e.input(t, good, "printf '%s\\n' '"+r1ProcSecret+"' >&2\nsleep 1\n"+
		"printf '{\"Version\":1,\"AccessKeyId\":\"ASIA"+"BBPROCKEY0000001\",\"SecretAccessKey\":\"%s\"}\\n' '"+r1ProcSecret+"'\n")
	bad := filepath.Join(e.home, "bad.sh")
	e.input(t, bad, "printf '{\"Version\":1,\"AccessKeyId\":\"ASIA"+"BBPROCKEY0000002\",\"SecretAccessKey\":\"%s' '"+r1ProcParse+"'\n")
	cfg, err := os.ReadFile(filepath.Join(e.home, ".aws", "config"))
	if err != nil {
		t.Fatal(err)
	}
	e.input(t, filepath.Join(e.home, ".aws", "config"), string(cfg)+"\n[profile bbproc]\ncredential_process = sh "+good+"\nregion = us-east-1\n"+
		"\n[profile bbprocbad]\ncredential_process = sh "+bad+"\nregion = us-east-1\n")
	out := e.run(t, "bill", "--profile-aws", "bbproc", "--data-source", "live")
	if strings.Contains(out, r1ProcSecret) {
		t.Errorf("the credential_process's secret was printed (its stderr, before the credentials were held): %s", around(out, r1ProcSecret))
	}
	if e.aws.signed("ASIA"+"BBPROCKEY0000001") == 0 {
		t.Errorf("control: no request was signed with the credential_process's credentials:\n%.1200s", out)
	}
	if out := e.run(t, "bill", "--profile-aws", "bbprocbad", "--data-source", "live"); strings.Contains(out, r1ProcParse) {
		t.Errorf("a credential_process answer that does not parse was printed in the error: %s", around(out, r1ProcParse))
	}
}

func TestR1CostCacheAnswersOnlyTheRequest(t *testing.T) {
	t.Parallel()
	const p = "2026-09-01:2026-10-01"
	t.Run("granularity", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, srcEnv)
		e.aws.quiet = true
		for _, g := range []string{"MONTHLY", "DAILY"} {
			e.run(t, "sync", "--from", "2026-09-01", "--to", "2026-10-01", "--skip-inventory", "--granularity", g, "--json")
		}
		out := e.run(t, "bill", "--period", p, "--data-source", "local", "--json")
		if got, ok := r1Total(t, out); !ok || got != 999.99 {
			t.Errorf("a MONTHLY bill after a MONTHLY and a DAILY sync: want 999.99 (the one monthly bucket), got %v:\n%.800s", got, out)
		}
	})
	t.Run("interval", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, srcEnv)
		e.aws.set(func(f *fakeAWS) { f.quiet, f.perDay = true, 1 })
		if got, ok := r1Total(t, e.run(t, "bill", "--period", "2026-09-01:2026-09-15", "--data-source", "live", "--json")); !ok || got != 14 {
			t.Fatalf("control: half of September is $14, got %v", got)
		}
		out := e.run(t, "bill", "--period", p, "--json")
		if got, ok := r1Total(t, out); !ok || got != 30 {
			t.Errorf("September after a cached half-September answer: want 30, got %v:\n%.800s", got, out)
		}
	})
	t.Run("estimated", func(t *testing.T) {
		t.Parallel()
		// A period AWS marks Estimated (the current month) changes until it
		// closes: an automatic read never reuses the stored estimate, an
		// explicit local read does; a final period is reused.
		for _, est := range []bool{true, false} {
			e := newEnv(t, srcEnv)
			e.aws.set(func(f *fakeAWS) {
				f.quiet, f.estimated = true, est
				f.amountBySigner = map[string]float64{bbEnvKey: 100}
			})
			if got, ok := r1Total(t, e.run(t, "bill", "--period", p, "--data-source", "live", "--json")); !ok || got != 100 {
				t.Fatalf("control: the first answer is $100, got %v", got)
			}
			e.aws.set(func(f *fakeAWS) { f.amountBySigner = map[string]float64{bbEnvKey: 150} })
			auto, _ := r1Total(t, e.run(t, "bill", "--period", p, "--json"))
			local, _ := r1Total(t, e.run(t, "bill", "--period", p, "--data-source", "local", "--json"))
			switch {
			case est && auto != 150:
				t.Errorf("an automatic read reused a stored estimated period: want the live $150, got %v", auto)
			case !est && auto != 100:
				t.Errorf("an automatic read did not reuse a stored final period: want the stored $100, got %v", auto)
			}
			if local != 100 && !(est && local == 150) {
				t.Errorf("estimated=%v: a local read answered %v, want a stored answer", est, local)
			}
		}
	})
	t.Run("account", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, srcProfileFlag)
		e.aws.set(func(f *fakeAWS) {
			f.quiet = true
			f.callerBySigner = map[string]string{bbFileKey: "111111111111", bbDefKey: "444455556666"}
			f.amountBySigner = map[string]float64{bbFileKey: 100, bbDefKey: 200}
		})
		if got, ok := r1Total(t, e.run(t, "bill", "--profile-aws", "bbfile", "--period", p, "--data-source", "live", "--json")); !ok || got != 100 {
			t.Fatalf("control: tenant A's bill is $100, got %v", got)
		}
		out := e.run(t, "bill", "--profile-aws", "default", "--period", p, "--json")
		if got, ok := r1Total(t, out); !ok || got != 200 {
			t.Errorf("tenant B's bill after tenant A's was cached: want 200, got %v:\n%.800s", got, out)
		}
		if out, code := e.runCode(t, "", bbCLI, "bill", "--period", p, "--data-source", "local", "--json"); code == 0 {
			t.Errorf("a local read with two accounts cached and none chosen answered anyway:\n%.800s", out)
		}
		e.run(t, "sync", "--profile-aws", "bbfile", "--skip-cost", "--json")
		rank := e.run(t, "waste", "rank", "--profile-aws", "default", "--json")
		if r1JSONHas(rank, "account_id", "111111111111") || !r1JSONHas(rank, "account_id", "444455556666") {
			t.Errorf("tenant B's waste after tenant A's inventory was synced lists tenant A's resources:\n%.1200s", rank)
		}
	})
}

func TestR1BillGroupByFetchesThatDimension(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.aws.quiet = true
	for by, want := range map[string]string{"usage-type": "HQ-UsageType-BoxUsage", "region": "us-west-2"} {
		out := e.run(t, "bill", "--period", "2026-09-01:2026-10-01", "--data-source", "live", "--group-by", by, "--json")
		if !r1JSONHas(out, "key", want) || strings.Contains(out, "(unattributed)") {
			t.Errorf("bill --group-by %s did not break the bill down by %s:\n%.800s", by, want, out)
		}
	}
}

func TestR1BillingReadFailureIsNeverAnEmptyBill(t *testing.T) {
	t.Parallel()
	t.Run("ask", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, srcEnv)
		e.aws.set(func(f *fakeAWS) { f.quiet, f.mode = true, modeDeny })
		for _, q := range []string{"what's my total", "which account costs the most", "top services", "what changed since last month"} {
			if out, code := e.runCode(t, "", bbCLI, "ask", q, "--data-source", "live", "--json"); code == 0 {
				t.Errorf("ask %q with Cost Explorer refusing exited 0:\n%.600s", q, out)
			}
		}
	})
	t.Run("prior-period", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, srcEnv)
		e.aws.set(func(f *fakeAWS) { f.quiet, f.ceFailStart = true, map[string]bool{"2026-08-01": true} })
		for _, args := range [][]string{
			{"consolidated", "--period", "2026-09-01:2026-10-01", "--data-source", "live", "--json"},
			{"report", "--period", "2026-09-01:2026-10-01", "--data-source", "live"},
		} {
			if out, code := e.runCode(t, "", bbCLI, args...); code == 0 {
				t.Errorf("%v with the prior period's read failing exited 0:\n%.800s", args, out)
			}
		}
	})
	t.Run("stored-incomplete-scan", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, srcEnv)
		e.aws.set(func(f *fakeAWS) { f.quiet, f.cwDeny = true, true })
		e.run(t, "sync", "--skip-cost", "--json")
		if out, code := e.runCode(t, "", bbCLI, "waste", "idle", "--data-source", "local", "--json"); code == 0 {
			t.Errorf("waste idle read a stored scan whose idle check failed as complete:\n%.800s", out)
		}
	})
	t.Run("accounts-denied", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, srcEnv)
		e.aws.set(func(f *fakeAWS) { f.quiet, f.mode = true, modeDeny })
		if out, code := e.runCode(t, "", bbCLI, "accounts", "list", "--data-source", "live", "--json"); code == 0 {
			t.Errorf("accounts list with Organizations refusing exited 0:\n%.600s", out)
		}
	})
}

func TestR1AskTransferKeepsTheCallersScope(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.aws.quiet = true
	db := filepath.Join(e.home, "tenant.db")
	out := e.run(t, "ask", "where is my data transfer going", "--period", "last-month", "--db", db, "--data-source", "live", "--json")
	if !r1JSONHas(out, "period", "last month") {
		t.Errorf("ask's transfer answer is not for the period asked (last month):\n%.800s", out)
	}
	if _, err := os.Stat(db); err != nil {
		t.Errorf("ask's transfer answer did not use the database asked for (--db): %v", err)
	}
}

func TestR1CopiedSnapshotIsNotOrphanWaste(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.aws.set(func(f *fakeAWS) { f.quiet, f.snapCopy = true, true })
	out := e.run(t, "waste", "snapshots", "--data-source", "live", "--json")
	if strings.Contains(out, "snap-0hqcopy") {
		t.Errorf("a recent copied snapshot is reported as waste from its (arbitrary) volume ID:\n%.1200s", out)
	}
}

func TestR1S3BucketRegionIsTheBucketsOwn(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.aws.quiet = true
	dbPath := filepath.Join(e.home, "r1.db")
	e.run(t, "sync", "--region", "us-east-1", "--skip-cost", "--db", dbPath, "--json")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var region string
	if err := db.QueryRow(`SELECT COALESCE(region, '') FROM inventory WHERE resource_type = 's3' AND resource_id = 'hq-bucket'`).Scan(&region); err != nil {
		t.Fatalf("the sync stored no hq-bucket row: %v", err)
	}
	if region != "eu-west-1" {
		t.Errorf("an S3 bucket in eu-west-1 is stored as region %q (the scan region, not the bucket's)", region)
	}
}
