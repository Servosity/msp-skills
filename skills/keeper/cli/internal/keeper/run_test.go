// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeper

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"keeper-pp-cli/internal/client"
)

// fakePoster answers each command from a script and records what was sent.
type fakePoster struct {
	answers map[string]answer // keyed by command name
	sent    []string
}

type answer struct {
	status int
	body   string
}

func (f *fakePoster) Post(_ context.Context, path string, body any) (json.RawMessage, int, error) {
	if path != ExecutePath {
		return nil, 0, errors.New("unexpected path " + path)
	}
	cmd := body.(map[string]string)["command"]
	f.sent = append(f.sent, cmd)
	name := strings.Fields(cmd)[0]
	a, ok := f.answers[name]
	if !ok {
		a = answer{500, `{"status":"error","error":"unscripted"}`}
	}
	if a.status >= 400 {
		return nil, a.status, &client.APIError{Method: "POST", Path: path, StatusCode: a.status, Body: a.body}
	}
	return json.RawMessage(a.body), a.status, nil
}

func TestRunProjectsOntoAllowlistedFields(t *testing.T) {
	f := &fakePoster{answers: map[string]answer{"enterprise-info": {200, `{"status":"success","command":"enterprise-info","data":[
		{"user_id":1,"email":"a@x.co","status":"Active","2fa_enabled":true,"roles":["Admins"],"surprise":"drop me","teams":[{"team_uid":"t"}]},
		{"user_id":2,"email":"b@x.co","status":"Invited"}]}`}}}
	rows, err := Run(context.Background(), f, Users())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["email"] != "a@x.co" || rows[0]["2fa_enabled"] != true {
		t.Fatalf("rows: %v", rows)
	}
	if _, ok := rows[0]["surprise"]; ok {
		t.Fatal("a field outside the allowlist was kept")
	}
	if _, ok := rows[1]["2fa_enabled"]; ok {
		t.Fatal("absent 2fa_enabled was invented")
	}
}

// Every non-report answer is an error, never zero rows.
func TestRunFailsClosedOnEveryOtherShape(t *testing.T) {
	cases := map[string]answer{
		"error status":       {200, `{"status":"error","error":"boom"}`},
		"warning":            {200, `{"status":"warning","message":"skipping"}`},
		"partial":            {200, `{"status":"partial_success","message":"x","data":null}`},
		"log message":        {200, `{"status":"success","command":"enterprise-info","message":"No users","data":null}`},
		"missing data":       {200, `{"status":"success","command":"enterprise-info"}`},
		"string data":        {200, `{"status":"success","command":"enterprise-info","data":"[]"}`},
		"object data":        {200, `{"status":"success","command":"enterprise-info","data":{"a":1}}`},
		"error with success": {200, `{"status":"success","error":"half","data":[]}`},
		"non-object row":     {200, `{"status":"success","data":[1]}`},
		"null row":           {200, `{"status":"success","data":[null]}`},
		"encrypted":          {200, `gAAAAABk...`},
		"empty body":         {200, ``},
		"http 403":           {403, `{"status":"error","error":"Not permitted to perform this function"}`},
		"http 500 non-json":  {500, `<html>oops</html>`},
		"missing status":     {200, `{"data":[]}`},
	}
	for name, a := range cases {
		f := &fakePoster{answers: map[string]answer{"enterprise-info": a}}
		rows, err := Run(context.Background(), f, Users())
		if err == nil {
			t.Errorf("%s: no error (rows %v)", name, rows)
		}
	}
	f := &fakePoster{answers: map[string]answer{"enterprise-info": {200, `{"status":"success","data":[]}`}}}
	if rows, err := Run(context.Background(), f, Users()); err != nil || len(rows) != 0 {
		t.Fatalf("a real empty report: %v %v", rows, err)
	}
}

func TestSecurityAuditErrorReportIsNotScores(t *testing.T) {
	f := &fakePoster{answers: map[string]answer{"security-audit-report": {200, `{"status":"success","command":"security-audit-report","data":[{"vault_owner":"a@x.co","error_message":"Decryption fail"}]}`}}}
	_, err := Run(context.Background(), f, SecurityAudit(false))
	if !errors.Is(err, ErrSecurityAuditErrorReport) {
		t.Fatalf("want the error-report error, got %v", err)
	}
}

// A record-content field or a secret-shaped value anywhere refuses the whole
// response; nothing from it is returned.
func TestRecordContentIsRefused(t *testing.T) {
	for _, row := range []string{
		`{"email":"a@x.co","password":"hunter2"}`,
		`{"email":"a@x.co","Notes":"x"}`,
		`{"email":"a@x.co","custom_fields":[]}`,
		`{"email":"a@x.co","name":"otpauth://totp/x?secret=ABC"}`,
		`{"email":"a@x.co","teams":["-----BEGIN RSA PRIVATE KEY-----"]}`,
		`{"email":"a@x.co","extra":{"deep":"otpauth://totp/y"}}`,
	} {
		f := &fakePoster{answers: map[string]answer{"enterprise-info": {200, `{"status":"success","data":[` + row + `]}`}}}
		rows, err := Run(context.Background(), f, Users())
		if !errors.Is(err, ErrRecordContent) || rows != nil {
			t.Errorf("%s: want ErrRecordContent, got %v %v", row, rows, err)
		}
	}
}

func TestRolePostureKeepsOnlyTheTwoFAEnforcement(t *testing.T) {
	f := &fakePoster{answers: map[string]answer{"enterprise-info": {200, `{"status":"success","data":[{"role_id":7,"name":"Admins","admin":true,"enforcements":{"require_two_factor":true,"restrict_ip_addresses":"10.0.0.0/8","master_password_minimum_length":12}}]}`}}}
	rows, err := Run(context.Background(), f, RolePosture())
	if err != nil {
		t.Fatal(err)
	}
	enf := rows[0]["enforcements"].(map[string]any)
	if len(enf) != 1 || enf["require_two_factor"] != true {
		t.Fatalf("enforcements: %v", enf)
	}
}

// When the session cannot be confirmed in the MSP context, nothing is read.
func TestUnconfirmedContextReadsNothing(t *testing.T) {
	for name, a := range map[string]answer{
		"forbidden":  {403, `{"status":"error","error":"Not permitted to perform this function"}`},
		"no session": {401, `{"status":"error","error":"No active session. Please log in through the CLI first."}`},
	} {
		f := &fakePoster{answers: map[string]answer{"switch-to-msp": a}}
		read := false
		if err := WithCompany(context.Background(), f, "", nil, func() error { read = true; return nil }); err == nil || read {
			t.Errorf("%s: err %v read %v", name, err, read)
		}
	}
	f := &fakePoster{answers: map[string]answer{
		"switch-to-msp": {500, `{"status":"error","error":"Already MSP"}`},
		"switch-to-mc":  {200, `{"status":"success","message":"something else","data":null}`},
	}}
	read := false
	if err := WithCompany(context.Background(), f, "4421", nil, func() error { read = true; return nil }); err == nil || read {
		t.Fatalf("an unconfirmed switch read anyway: %v", err)
	}
}
