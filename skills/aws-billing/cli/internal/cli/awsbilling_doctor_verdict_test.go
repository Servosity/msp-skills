// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"aws-billing-pp-cli/internal/cliutil/testenv"
)

// doctor --json shows its own auth, credentials and aws_credentials
// verdicts, and with no AWS credential the credentials row is a negative
// verdict an operator and check_doctor_truth.py can read.
func TestDoctorJSONShowsItsOwnVerdicts(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent")
	for _, k := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE"} {
		t.Setenv(k, "")
	}
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"doctor", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out.String())
	}
	for k, prefix := range map[string]string{"credentials": "ERROR not verified", "aws_credentials": "ERROR no AWS credentials", "auth": "resolved by the AWS SDK"} {
		if s, _ := report[k].(string); !strings.HasPrefix(s, prefix) {
			t.Errorf("%s = %q, want a verdict starting %q", k, s, prefix)
		}
	}
}
