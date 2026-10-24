// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
)

// CIPP hardcodes HTTP 200 on several write entrypoints and reports the failure
// as plain prose inside the `Results` envelope. Invoke-EditUser,
// Invoke-AddScheduledItem and Invoke-ExecOffboardUser are the confirmed cases,
// and the pattern runs through the wider Exec*/Add*/Edit* families. A
// status-code check alone therefore reports success on failure — the CLI would
// exit 0 after a write that did nothing.
//
// The generated detectPartialFailure() in helpers.go only recognizes the Google
// Ads `partialFailureError` shape (an object with message/code), so it never
// matches CIPP's array-of-strings and is structurally dead for this API. This
// file supplies the CIPP-shaped detector and helpers.go delegates to it.
//
// Kept in its own file so a regen preserves it whole: only the one-line
// delegation inside helpers.go is an edit to generated code, recorded in the
// hand-fix ledger.

// Hand-fix (handfixes.json cipp-results-failure-vocabulary). There is no field
// list and no prefix rule: every string in a result, under any key at any
// depth, is read, and the vocabulary below is matched anywhere in it on word
// boundaries (plus camelCase codes such as AuthorizationFailed or
// Request_ResourceNotFound). A false failure is the safe side: a conflicting
// or failed write is an error and is never resent automatically.
var (
	cippFailureWords = regexp.MustCompile(`(?i)\b(fail|fails|failed|failing|failure|failures|error|errors|errored|could not|couldn't|unable|cannot|can't|can not|not permitted|not allowed|denied|forbidden|unauthori[sz]ed|insufficient|already exists|does not exist|not found|invalid|rejected|refused|blocked|exception|timed out|timeout|throttl\w*)\b|\w(failed|failure|error|denied|forbidden|invalid|notfound|exception)\b|\bnotfound\b`)
	cippSuccessWords = regexp.MustCompile(`(?i)\b(success|successful|successfully|succeeded|completed|accepted)\b`)
	cippFailureState = regexp.MustCompile(`(?i)^\s*(error|errored|failed|failure|fail|denied|forbidden|unauthorized)\s*$`)
	cippSuccessState = regexp.MustCompile(`(?i)^\s*(success|succeeded|successful|ok|completed|complete)\s*$`)
)

// cippVerdict is the outcome of one CIPP write result: success, failure, or
// conflicting (a success signal and a failure signal in the same result). A
// conflicting write is reported as an error and is never resent automatically,
// because it may already have taken effect.
type cippVerdict int

const (
	cippSuccess cippVerdict = iota
	cippFailure
	cippConflicting
)

// cippSignals collects every success and failure signal found in a value, and
// what raised each failure (capped, for the error message).
type cippSignals struct {
	fail, success bool
	flagged       []string
	// list: walking a list response, where failure wording counts only under
	// a status-like key (cippListWordKey) or as a bare message string.
	list bool
}

func (s cippSignals) verdict() cippVerdict {
	switch {
	case s.fail && s.success:
		return cippConflicting
	case s.fail:
		return cippFailure
	}
	return cippSuccess
}

func (s *cippSignals) failed(what string) {
	s.fail = true
	if len(what) > 300 {
		what = what[:300] + "..."
	}
	for _, f := range s.flagged {
		if f == what || strings.HasSuffix(f, ": "+what) {
			return
		}
	}
	if len(s.flagged) < 10 {
		s.flagged = append(s.flagged, what)
	}
}

// cippStateKey reports a key that carries a verdict word (state, status, ...).
func cippStateKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "state") || strings.Contains(k, "status")
}

// cippScan is the one document walker (hand-fix handfixes.json
// cipp-results-failure-vocabulary), used by write results (bulk, cipp_execute,
// every mutation command) and by list validation (fanout --save, the tenant
// list, GDAP) alike. cippScanDoc walks the WHOLE document: the envelope and
// every item, at any depth. Failure signals:
//   - failure wording in any string value, on word boundaries (in a list
//     response: only in a value under a status-like key, cippListWordKey, at
//     any depth below it, or in a bare message string, so a user named "Error
//     Reporting" is still a row);
//   - a populated key whose lowercased name contains error, exception or fault;
//   - a key named like success, succeeded or ok holding false (the boolean or
//     the string "false");
//   - a failure state under a state/status-like key, at any depth below it
//     ({"status":{"code":"error"}}).
//
// Success signals are the mirror images (success wording, a success state, a
// success/ok key holding true). Status-like ancestry starts at any key named
// like a status, except the document's own row container (a top-level array,
// or the Results/data/value/items array of a top-level envelope), whose rows
// are data. There is no exemption here: only the tenant list's validation
// passes rowExcept (cippResponseFailure).
func cippScan(v any, key string, sig *cippSignals) {
	cippScanValue(v, key, sig, cippListWordKey(key))
}

// cippScanDoc walks a whole response document. rowExcept, when set, names the
// top-level keys (lowercased) of each row left out of the walk.
func cippScanDoc(doc any, sig *cippSignals, rowExcept map[string]bool) {
	switch x := doc.(type) {
	case []any:
		cippScanRows(x, "", sig, rowExcept)
	case map[string]any:
		for k, child := range x {
			if rows, isRows := child.([]any); isRows && cippEnvelopeKey(k) {
				cippScanRows(rows, k, sig, rowExcept)
				continue
			}
			cippScanEntry(k, child, sig, false)
		}
	default:
		cippScanValue(doc, "", sig, false)
	}
}

func cippScanRows(rows []any, key string, sig *cippSignals, rowExcept map[string]bool) {
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			for k, child := range m {
				if rowExcept[strings.ToLower(k)] {
					continue
				}
				cippScanEntry(k, child, sig, false)
			}
			continue
		}
		// A bare value in the row container is a message, not a row.
		cippScanValue(r, key, sig, key == "" || cippListWordKey(key))
	}
}

// cippScanEntry walks one object member: its key's structural signal, then its
// value, which is under a status-like key when this key, or any above it, is
// one.
func cippScanEntry(k string, child any, sig *cippSignals, inStatus bool) {
	// Structural signal: a populated error field is a failure, whatever its
	// wording ("HTTP 403").
	if cippErrorKey(k) && cippPopulated(child) {
		sig.failed(k + ": " + cippEntryText(child))
	}
	cippScanValue(child, k, sig, inStatus || cippListWordKey(k))
}

func cippScanValue(v any, key string, sig *cippSignals, inStatus bool) {
	switch x := v.(type) {
	case string:
		if cippStateKey(key) || inStatus {
			if cippFailureState.MatchString(x) {
				sig.failed(key + ": " + x)
			}
			if cippSuccessState.MatchString(x) {
				sig.success = true
			}
		}
		if cippOKKey(key) {
			switch strings.ToLower(strings.TrimSpace(x)) {
			case "false":
				sig.failed(key + ": " + x)
			case "true":
				sig.success = true
			}
		}
		if cippFailureWords.MatchString(x) && (!sig.list || inStatus || key == "") {
			sig.failed(x)
		}
		if cippSuccessWords.MatchString(x) {
			sig.success = true
		}
	case bool:
		if cippOKKey(key) {
			if x {
				sig.success = true
			} else {
				sig.failed(key + ": false")
			}
		}
	case []any:
		for _, it := range x {
			cippScanValue(it, key, sig, inStatus)
		}
	case map[string]any:
		for k, child := range x {
			cippScanEntry(k, child, sig, inStatus)
		}
	}
}

// cippEnvelopeKey reports a top-level key that holds a list's rows.
func cippEnvelopeKey(key string) bool {
	for _, want := range tenantEnvelopeKeys {
		if strings.EqualFold(key, want) {
			return true
		}
	}
	return false
}

// cippErrorKey reports a key that carries an error (hand-fix handfixes.json
// cipp-results-failure-vocabulary): its lowercased name contains error,
// exception or fault ("default", as in defaultDomainName, is not a fault).
func cippErrorKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "error") || strings.Contains(k, "exception") ||
		strings.Contains(strings.ReplaceAll(k, "default", ""), "fault")
}

// cippPopulated reports a value that carries something: a non-blank string, a
// non-zero number, true, a non-empty object or a non-empty array. A zero
// number and false are empty, so an "errorCode": 0 (Graph's "no error") is
// not a failure.
func cippPopulated(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case bool:
		return x
	case float64:
		return x != 0
	case json.Number:
		f, err := x.Float64()
		return err != nil || f != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

// cippListWordKeyParts mark a status-like key in a list response: failure
// wording under one of these refuses the list; under any other key (a name, a
// description) it is row data.
var cippListWordKeyParts = []string{"state", "status", "result", "message", "msg", "text", "detail", "reason", "fail"}

func cippListWordKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range cippListWordKeyParts {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

// cippOKKey reports a key named like success, succeeded or ok: "success" or
// "succeeded" anywhere in its lowercased name, or "ok" as a whole word of it
// (isOk, OK, ok_status; not outlook or token).
func cippOKKey(key string) bool {
	k := strings.ToLower(key)
	if strings.Contains(k, "success") || strings.Contains(k, "succeeded") {
		return true
	}
	for _, word := range cippKeyWords(key) {
		if word == "ok" {
			return true
		}
	}
	return false
}

// cippKeyWords splits a key into lowercased words at camelCase humps and at
// anything that is not a letter or digit.
func cippKeyWords(key string) []string {
	var words []string
	var cur []rune
	runes := []rune(key)
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	for i, r := range runes {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
			continue
		case unicode.IsUpper(r) && i > 0 && (unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]) && unicode.IsUpper(runes[i-1]))):
			flush()
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// errCippDuplicateKey marks a response with a duplicate object key: a decoder
// keeps only one of the two values, so the rest of the response cannot be
// trusted ({"Results":["Failed"],"Results":["success"]}).
var errCippDuplicateKey = errors.New("CIPP returned a response with a duplicate key")

// cippDocument decodes a response body for cippScanDoc with cippDecodeStrict:
// any JSON value, or the text itself when the body is not JSON. ok is false
// for an empty body; err is set (errCippDuplicateKey) when an object at any
// depth repeats a key.
func cippDocument(data []byte) (doc any, ok bool, err error) {
	if strings.TrimSpace(string(data)) == "" {
		return nil, false, nil
	}
	doc, err = cippDecodeStrict(data)
	if errors.Is(err, errCippDuplicateKey) {
		return nil, true, err
	}
	if err != nil {
		return string(data), true, nil
	}
	return doc, true, nil
}

// cippDecodeStrict decodes one JSON value (numbers as json.Number) and refuses
// a duplicate object key at any depth, compared case-insensitively because
// every lookup here is (hand-fix handfixes.json
// cipp-results-failure-vocabulary).
func cippDecodeStrict(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := cippDecodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data after the JSON value")
	}
	return v, nil
}

func cippDecodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, isDelim := tok.(json.Delim)
	if !isDelim {
		return tok, nil
	}
	switch d {
	case '{':
		m := map[string]any{}
		seen := map[string]bool{}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			k, _ := kt.(string)
			if seen[strings.ToLower(k)] {
				return nil, fmt.Errorf("%w %q", errCippDuplicateKey, k)
			}
			seen[strings.ToLower(k)] = true
			v, err := cippDecodeValue(dec)
			if err != nil {
				return nil, err
			}
			m[k] = v
		}
		_, err := dec.Token()
		return m, err
	case '[':
		arr := []any{}
		for dec.More() {
			v, err := cippDecodeValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token()
		return arr, err
	}
	return nil, fmt.Errorf("unexpected %v", d)
}

// cippResponseFailure walks a whole list response document and returns what
// raised its failure signals, or "" when there are none. List validation
// refuses the whole response on any of them. rowExcept is set only by the
// tenant list's validation (tenantRecordFields).
func cippResponseFailure(doc any, rowExcept map[string]bool) string {
	sig := cippSignals{list: true}
	cippScanDoc(doc, &sig, rowExcept)
	if !sig.fail {
		return ""
	}
	return strings.Join(sig.flagged, "; ")
}

// interpretCippResults normalizes a CIPP `Results` payload into strings,
// flags every entry that carries a failure signal, and decides the whole
// payload: any failure signal together with any success signal anywhere in it
// is conflicting; failure signals alone are a failure; success only when no
// failure signal exists anywhere.
func interpretCippResults(raw any) (results []string, flagged []string, verdict cippVerdict) {
	var entries []any
	switch v := raw.(type) {
	case nil:
		return nil, nil, cippSuccess
	case []any:
		entries = v
	default:
		entries = []any{v}
	}
	var all cippSignals
	results = make([]string, 0, len(entries))
	for _, e := range entries {
		s, ok := e.(string)
		if !ok {
			b, err := json.Marshal(e)
			if err != nil {
				continue
			}
			s = string(b)
		}
		var sig cippSignals
		cippScan(e, "Results", &sig)
		results = append(results, s)
		if sig.fail {
			flagged = append(flagged, s)
		}
		all.fail = all.fail || sig.fail
		all.success = all.success || sig.success
	}
	return results, flagged, all.verdict()
}

// cippResultsVerdict decides a 2xx CIPP write response body by walking the
// whole document (cippScan): the envelope beside Results counts as much as
// Results does ({"Results":["success"],"error":"HTTP 403"} is conflicting).
func cippResultsVerdict(data []byte) (cippVerdict, string, []string) {
	doc, ok, err := cippDocument(data)
	if err != nil {
		return cippFailure, fmt.Sprintf("CIPP returned HTTP 2xx with a response that cannot be read safely: %v", err), nil
	}
	if !ok {
		return cippSuccess, "", nil
	}
	var sig cippSignals
	cippScanDoc(doc, &sig, nil)
	var results []string
	if top, isObj := doc.(map[string]any); isObj {
		for k, v := range top {
			if strings.EqualFold(k, "Results") {
				results, _, _ = interpretCippResults(v)
				break
			}
		}
	}
	switch sig.verdict() {
	case cippConflicting:
		return cippConflicting, fmt.Sprintf("CIPP returned HTTP 2xx with both success and failure in its response; check whether the write took effect before running it again: %s",
			strings.Join(sig.flagged, "; ")), results
	case cippFailure:
		return cippFailure, fmt.Sprintf("CIPP returned HTTP 2xx reporting failure: %s", strings.Join(sig.flagged, "; ")), results
	}
	return cippSuccess, "", results
}

// detectCippResultsFailure inspects a 2xx CIPP response body for a failure or
// a conflict inside the `Results` envelope. Returns nil on success.
func detectCippResultsFailure(data []byte) *partialFailureReport {
	verdict, msg, results := cippResultsVerdict(data)
	if verdict == cippSuccess {
		return nil
	}
	return &partialFailureReport{Field: "Results", Message: msg, Details: results}
}

// CippResultsFailure is detectCippResultsFailure for callers outside this
// package (the MCP executor): the failure message when a 2xx CIPP body reports
// a failure inside Results, else "".
func CippResultsFailure(data []byte) string {
	if r := detectCippResultsFailure(data); r != nil {
		return r.Message
	}
	return ""
}
