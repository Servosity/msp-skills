// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Decoder for ESET policy feature configuration blobs.
//
// A policy's features[].configuration is a protobuf Any whose "@type" names a
// PolicyData message and whose "data" is base64 (sometimes base64 of base64).
// The decoded bytes start with a JSON document; endpoint policies append an
// `ar` archive whose *.lzma members (endpoint.lzma) hold the product settings
// as LZMA-compressed JSON. Algorithm follows Fenrindale/eset-protect-mcp
// (src/eset-client.ts decodePolicyDataBlob / parseArArchive).
package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ulikunitz/xz/lzma"
)

const (
	policyDataMaxLayers = 5
	// Decompression-bomb guards: the LZMA header's declared dictionary is
	// allocated up front, so cap it, and cap total output across members.
	policyLZMAMaxDict  = 64 << 20
	policyLZMAMaxBytes = 64 << 20
)

var base64ishRE = regexp.MustCompile(`^[A-Za-z0-9+/=_-]{16,}$`)

type policyArchiveMember struct {
	Name              string          `json:"name"`
	Size              int             `json:"size"`
	DecompressedBytes int             `json:"decompressed_bytes,omitempty"`
	Decoded           json.RawMessage `json:"decoded,omitempty"`
	Error             string          `json:"error,omitempty"`
}

type decodedPolicyData struct {
	Path           string                `json:"path"`
	Type           string                `json:"type"`
	Product        any                   `json:"product,omitempty"`
	Layers         int                   `json:"decoding_layers,omitempty"`
	Decoded        json.RawMessage       `json:"decoded,omitempty"`
	TrailingBytes  int                   `json:"trailing_bytes,omitempty"`
	ArchiveMembers []policyArchiveMember `json:"archive_members,omitempty"`
	ArchiveError   string                `json:"archive_error,omitempty"`
	Error          string                `json:"error,omitempty"`
}

// policyDecodeErrors lists every blob, archive and member decode failure, so
// a search over the decoded tree can report that it was incomplete.
func policyDecodeErrors(found []decodedPolicyData) []string {
	var errs []string
	for _, d := range found {
		if d.Error != "" {
			errs = append(errs, d.Path+": "+d.Error)
		}
		if d.ArchiveError != "" {
			errs = append(errs, d.Path+" archive: "+d.ArchiveError)
		}
		for _, m := range d.ArchiveMembers {
			if m.Error != "" {
				errs = append(errs, d.Path+" member "+m.Name+": "+m.Error)
			}
		}
	}
	return errs
}

func normalizeBase64(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("-", "+", "_", "/").Replace(s)
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return s
}

// jsonPrefix parses the longest leading JSON value and returns the bytes that
// follow it (the ar archive on endpoint policies).
func jsonPrefix(b []byte) (json.RawMessage, []byte, error) {
	trimmed := bytes.TrimLeft(b, " \t\r\n")
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, nil, err
	}
	return raw, trimmed[dec.InputOffset():], nil
}

// decodePolicyDataBlob peels up to five base64 layers until a JSON value
// appears, then decodes any trailing ar archive, with its own output budget.
func decodePolicyDataBlob(data string) (decodedPolicyData, error) {
	budget := policyLZMAMaxBytes
	return decodePolicyDataBlobBudget(data, &budget)
}

// decodePolicyDataBlobBudget decodes one blob, drawing LZMA output from a
// budget shared with every other blob of the same policy.
func decodePolicyDataBlobBudget(data string, budget *int) (decodedPolicyData, error) {
	var out decodedPolicyData
	current := data
	var lastErr error
	for layer := 1; layer <= policyDataMaxLayers; layer++ {
		raw, err := base64.StdEncoding.DecodeString(normalizeBase64(current))
		if err != nil {
			return out, fmt.Errorf("layer %d is not base64: %w", layer, err)
		}
		value, trailing, err := jsonPrefix(raw)
		if err != nil {
			lastErr = err
			text := strings.TrimSpace(string(raw))
			if !base64ishRE.MatchString(text) {
				return out, fmt.Errorf("layer %d decoded to neither JSON nor base64: %w", layer, err)
			}
			current = text
			continue
		}
		var s string
		if json.Unmarshal(value, &s) == nil && base64ishRE.MatchString(strings.TrimSpace(s)) {
			current = s
			continue
		}
		out.Layers = layer
		out.Decoded = value
		trailing = bytes.TrimLeft(trailing, " \t\r\n")
		out.TrailingBytes = len(trailing)
		out.ArchiveMembers, out.ArchiveError = decodeArArchiveBudget(trailing, budget)
		return out, nil
	}
	if lastErr == nil {
		lastErr = errors.New("still base64 after 5 layers")
	}
	return out, lastErr
}

// decodeArArchive parses the ar archive that follows the JSON header. Members
// decoded before a malformed header or truncated body are kept, and the
// problem is returned so the caller never mistakes a partial archive for a
// complete one.
func decodeArArchive(b []byte) ([]policyArchiveMember, string) {
	budget := policyLZMAMaxBytes
	return decodeArArchiveBudget(b, &budget)
}

func decodeArArchiveBudget(b []byte, budget *int) ([]policyArchiveMember, string) {
	const magic = "!<arch>\n"
	i := bytes.Index(b, []byte(magic))
	if i < 0 {
		if len(bytes.TrimSpace(b)) > 0 {
			return nil, fmt.Sprintf("%d trailing bytes after the JSON header are not an ar archive", len(b))
		}
		return nil, ""
	}
	var members []policyArchiveMember
	skipped := ""
	if n := len(bytes.TrimSpace(b[:i])); n > 0 {
		skipped = fmt.Sprintf("%d unexpected bytes before the archive marker were ignored", n)
	}
	cur := i + len(magic)
	for cur < len(b) {
		if cur+60 > len(b) {
			return members, fmt.Sprintf("truncated member header at offset %d", cur)
		}
		hdr := b[cur : cur+60]
		if string(hdr[58:60]) != "`\n" {
			return members, fmt.Sprintf("bad member header trailer at offset %d", cur)
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(hdr[0:16])), "/")
		size, err := strconv.Atoi(strings.TrimSpace(string(hdr[48:58])))
		if err != nil || size < 0 {
			return members, fmt.Sprintf("bad size for member %q at offset %d", name, cur)
		}
		if cur+60+size > len(b) {
			return members, fmt.Sprintf("member %q is truncated: header says %d bytes, %d remain", name, size, len(b)-cur-60)
		}
		body := b[cur+60 : cur+60+size]
		m := policyArchiveMember{Name: name, Size: size}
		if strings.HasSuffix(strings.ToLower(name), ".lzma") {
			*budget -= decodeLZMAMember(&m, body, *budget)
		}
		members = append(members, m)
		cur += 60 + size + size%2
	}
	return members, skipped
}

// decodeLZMAMember decodes one *.lzma member within the remaining output
// budget and returns the bytes it consumed from that budget.
func decodeLZMAMember(m *policyArchiveMember, body []byte, budget int) int {
	if budget <= 0 {
		m.Error = fmt.Sprintf("total decoded size limit of %d bytes reached; member not decoded", policyLZMAMaxBytes)
		return 0
	}
	r, err := lzma.ReaderConfig{DictCap: policyLZMAMaxDict}.NewReader(bytes.NewReader(body))
	if err != nil {
		m.Error = "lzma: " + err.Error()
		return 0
	}
	plain, err := io.ReadAll(io.LimitReader(r, int64(budget)+1))
	if err != nil {
		m.Error = "lzma: " + err.Error()
		return len(plain)
	}
	if len(plain) > budget {
		m.Error = fmt.Sprintf("decoded settings exceed the %d byte limit; not decoded", policyLZMAMaxBytes)
		return budget
	}
	m.DecompressedBytes = len(plain)
	value, rest, err := jsonPrefix(plain)
	if err != nil {
		m.Error = "member is not JSON: " + err.Error()
		return len(plain)
	}
	m.Decoded = value
	if extra := len(bytes.TrimSpace(rest)); extra > 0 {
		m.Error = fmt.Sprintf("%d unexpected bytes after the settings JSON; the decoded value may be incomplete", extra)
	}
	return len(plain)
}

// esetDecodeExact decodes JSON into generic values with json.Number, so
// integers above 2^53 (feature ids, settings) survive a decode/encode round
// trip byte for byte instead of being rounded through float64.
func esetDecodeExact(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	// More() stops at a stray ']' or '}'; only io.EOF proves end of input.
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected data after the JSON value")
	}
	return nil
}

// collectPolicyData walks a policy document and decodes every PolicyData Any.
// All blobs share one decompression budget, so a policy with many small,
// highly compressed blobs cannot exhaust memory.
func collectPolicyData(v any, path string, out *[]decodedPolicyData) {
	budget := policyLZMAMaxBytes
	collectPolicyDataBudget(v, path, out, &budget)
}

func collectPolicyDataBudget(v any, path string, out *[]decodedPolicyData, budget *int) {
	switch t := v.(type) {
	case map[string]any:
		typ, _ := t["@type"].(string)
		if data, ok := t["data"].(string); ok && strings.Contains(typ, "PolicyData") {
			d, err := decodePolicyDataBlobBudget(data, budget)
			d.Path, d.Type, d.Product = path, typ, t["product"]
			if err != nil {
				d.Error = err.Error()
			}
			*out = append(*out, d)
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			collectPolicyDataBudget(t[k], path+"."+k, out, budget)
		}
	case []any:
		for i, item := range t {
			collectPolicyDataBudget(item, fmt.Sprintf("%s[%d]", path, i), out, budget)
		}
	}
}

// omitPolicyDataBlobs replaces raw base64 blobs so output stays readable.
func omitPolicyDataBlobs(v any) any {
	switch t := v.(type) {
	case map[string]any:
		typ, _ := t["@type"].(string)
		out := make(map[string]any, len(t))
		for k, item := range t {
			if s, ok := item.(string); ok && k == "data" && strings.Contains(typ, "PolicyData") {
				out[k] = fmt.Sprintf("[omitted %d base64 chars]", len(s))
				continue
			}
			out[k] = omitPolicyDataBlobs(item)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = omitPolicyDataBlobs(item)
		}
		return out
	default:
		return v
	}
}

type policyMatch struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// findInJSON returns leaf paths whose key or scalar value contains needle.
func findInJSON(v any, path, needle string, limit int, out *[]policyMatch) {
	if len(*out) >= limit {
		return
	}
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := path + "." + k
			if strings.Contains(strings.ToLower(k), needle) {
				*out = append(*out, policyMatch{Path: p, Value: t[k]})
				if len(*out) >= limit {
					return
				}
				continue
			}
			findInJSON(t[k], p, needle, limit, out)
		}
	case []any:
		for i, item := range t {
			findInJSON(item, fmt.Sprintf("%s[%d]", path, i), needle, limit, out)
		}
	default:
		if needle != "" && strings.Contains(strings.ToLower(fmt.Sprint(t)), needle) {
			*out = append(*out, policyMatch{Path: path, Value: t})
		}
	}
}

// valueAtPath resolves a dotted path with optional [n] indexes, e.g.
// "archive_members[0].decoded.Settings.Firewall".
func valueAtPath(v any, path string) (any, error) {
	cur := v
	for _, seg := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(path, "$"), "."), ".") {
		if seg == "" {
			continue
		}
		key, idx, indexed := seg, 0, false
		if open := strings.Index(seg, "["); open >= 0 && strings.HasSuffix(seg, "]") {
			n, err := strconv.Atoi(seg[open+1 : len(seg)-1])
			if err != nil || n < 0 {
				return nil, fmt.Errorf("bad index in %q (use a non-negative integer)", seg)
			}
			key, idx, indexed = seg[:open], n, true
		}
		if key != "" {
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%q is not an object", key)
			}
			if cur, ok = m[key]; !ok {
				return nil, fmt.Errorf("no key %q", key)
			}
		}
		if indexed {
			arr, ok := cur.([]any)
			if !ok || idx >= len(arr) {
				return nil, fmt.Errorf("index %d out of range at %q", idx, seg)
			}
			cur = arr[idx]
		}
	}
	return cur, nil
}
