// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): every credential file this program can read, for
// Declarations.Files (the kit holds them), and the startup refusal of one that
// does not parse. Pinned by this connector's handfixes.json under the
// "credkit-wiring" entry.

package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"zammad-pp-cli/internal/cliutil"
	"zammad-pp-cli/internal/config"
	"zammad-pp-cli/internal/credkit"
)

// credentialFilePaths is every TOML configuration or credentials file and
// every whole-file secret the command line or environment can name. The kit
// holds them all; selectedBaseURL says which one chooses the origin.
func credentialFilePaths(args []string) (tomlFiles, secretFiles []string) {
	configs := append(credkit.FlagValues(args, "--config"), "")
	homes := append(credkit.FlagValues(args, "--home"), "")
	for _, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue
		}
		for _, c := range configs {
			if path, explicit, err := config.ResolveConfigPath(c); err == nil {
				tomlFiles = append(tomlFiles, path)
				if creds, err := cliutil.CredentialsFilePathForConfig(path); explicit && err == nil {
					tomlFiles = append(tomlFiles, creds)
					secretFiles = append(secretFiles, cliutil.WebhookSecretPathNextTo(creds))
				}
			}
			if creds, err := cliutil.CredentialsFilePath(); err == nil {
				tomlFiles = append(tomlFiles, creds)
				secretFiles = append(secretFiles, cliutil.WebhookSecretPathNextTo(creds))
			}
			if legacy, err := config.LegacyConfigPath(); err == nil {
				tomlFiles = append(tomlFiles, legacy)
			}
		}
		restore()
	}
	return tomlFiles, secretFiles
}

// ShippedBaseURL is the base URL config.Load starts from.
const ShippedBaseURL = "https://your-instance.zammad.com/api/v1"

// BodyCredentialFields are the request body fields that carry a credential,
// for Declarations.CredentialFields and MCPArguments: a user's password (users
// create or update with a JSON body) and the webhook signature token.
var BodyCredentialFields = []string{"password", "signature_token"}

// HoldBodyCredentials holds what a request body carries in those fields, for
// a path that shows the body before the transport could hold it: a dry run,
// or a flag error before a piped body is read. A document or JSON Lines body
// is held by field, because holding all of it would withhold every answer
// that repeats a piece of it; one that does not parse is held whole.
func HoldBodyCredentials(body []byte) {
	var doc any
	if json.Unmarshal(body, &doc) == nil {
		holdCredentialFields(doc)
		return
	}
	var docs []any
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var d any
		if json.Unmarshal([]byte(line), &d) != nil {
			credkit.Hold("stdin", string(body))
			return
		}
		docs = append(docs, d)
	}
	holdCredentialFields(docs)
}

func holdCredentialFields(v any) {
	switch v := v.(type) {
	case []any:
		for _, e := range v {
			holdCredentialFields(e)
		}
	case map[string]any:
		for name, e := range v {
			if slices.ContainsFunc(BodyCredentialFields, func(f string) bool { return strings.EqualFold(f, name) }) {
				if s, ok := e.(string); ok {
					credkit.Hold("field:"+strings.ToLower(name), s)
				} else if b, err := json.Marshal(e); err == nil {
					credkit.Hold("field:"+strings.ToLower(name), string(b))
				}
				continue
			}
			holdCredentialFields(e)
		}
	}
}

// HoldStdinCredentials holds what a CLI command will read from stdin before
// anything is parsed, so a flag error cannot repeat it: all of it for auth set-token,
// whose stdin is the token; the credential fields of a --stdin or --input -
// body. Not Declarations.StdinFlags, which would hold all of every body. The
// bytes are given back on os.Stdin.
func HoldStdinCredentials(args []string) {
	isToken, isBody := stdinUse(args)
	if !isToken && !isBody {
		return
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	b, err := io.ReadAll(os.Stdin)
	if isToken || err != nil {
		credkit.Hold("stdin", string(b))
		credkit.Hold("stdin", strings.Split(string(b), "\n")...)
	} else {
		HoldBodyCredentials(b)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return
	}
	go func() {
		_, _ = w.Write(b)
		_ = w.Close()
	}()
	os.Stdin = r
}

// stdinUse reads the flags up to "--": a word that is a flag's value only
// makes it read more. feedback's --stdin is free text, not a request body.
func stdinUse(args []string) (isToken, isBody bool) {
	auth, setToken, first := false, false, ""
	for _, a := range args {
		if a == "--" {
			break
		}
		if first == "" && !strings.HasPrefix(a, "-") {
			first = a
		}
		auth = auth || a == "auth"
		setToken = setToken || a == "set-token"
		if a == "--stdin" {
			isBody = true
		} else if v, ok := strings.CutPrefix(a, "--stdin="); ok {
			on, err := strconv.ParseBool(v)
			isBody = isBody || on || err != nil
		}
	}
	isBody = isBody && first != "feedback"
	isBody = isBody || slices.Contains(credkit.FlagValues(args, "--input"), "-") || slices.Contains(credkit.FlagValues(args, "-i"), "-")
	return auth && setToken, isBody
}

// profileCredentialName is a run-profile field named for a credential.
var profileCredentialName = regexp.MustCompile(`(?i)token|secret|password|credential|api.?key|private.?key|authorization|cookie`)

// holdProfileCredentials reads profiles.json as a credential source: a value
// under a credential's name or under base-url, every duplicate included. Free text
// (a description, a URL in another field) is not a source: contract section 1. Not
// Declarations.Files: the kit holds every string of a nested JSON file as a
// credential, and ordinary values there (a flag's "true") make it refuse to start.
// profile.go's loader keeps only the allow-list out of the file.
func holdProfileCredentials(args []string) {
	var paths []string
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".zammad-cli", "profiles.json"))
	}
	for _, home := range append(credkit.FlagValues(args, "--home"), "") {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			paths = append(paths, filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	for _, path := range paths {
		if opened, err := cliutil.ReadOpenedConfig(path); err == nil {
			dec := json.NewDecoder(bytes.NewReader(opened.Data))
			dec.UseNumber()
			_ = holdProfileValue(dec, "") // held up to where a broken file stops
		}
	}
}

// holdProfileValue walks tokens: a decoded tree keeps only the last member of a
// duplicate key.
func holdProfileValue(dec *json.Decoder, key string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch x := tok.(type) {
	case json.Delim: // '{' or '[': a closing one is read below
		for dec.More() {
			if x == '{' {
				if tok, err = dec.Token(); err != nil {
					return err
				}
				key, _ = tok.(string)
			}
			if err := holdProfileValue(dec, key); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case json.Number:
		holdProfileString(key, x.String())
	case string:
		holdProfileString(key, x)
	}
	return nil
}

func holdProfileString(key, value string) {
	const source = "config:profiles"
	if profileCredentialName.MatchString(key) {
		credkit.Hold(source, value)
	} else if strings.EqualFold(strings.ReplaceAll(key, "_", "-"), "base-url") {
		holdBaseURL(source, value)
	}
}

// holdBaseURL holds what contract section 1 names inside a base URL: its password,
// or its user when it has no password (a token written as https://TOKEN@host), and
// every query value at any length, as the kit does for BaseURL.Env.
func holdBaseURL(source, raw string) {
	raw = strings.TrimSpace(raw)
	query := ""
	if u, err := url.Parse(raw); err == nil {
		query = u.RawQuery
		if u.User != nil {
			pass, ok := u.User.Password()
			if !ok {
				pass = u.User.Username()
			}
			credkit.Hold(source, pass)
		}
	} else {
		credkit.Hold(source, raw)
		_, query, _ = strings.Cut(raw, "?")
		query, _, _ = strings.Cut(query, "#")
	}
	for _, pair := range strings.FieldsFunc(query, func(r rune) bool { return r == '&' || r == ';' }) {
		_, v, _ := strings.Cut(pair, "=")
		credkit.Hold(source, v)
		if dec, err := url.QueryUnescape(v); err == nil {
			credkit.Hold(source, dec)
		}
	}
}

// originSetForKit marks ZAMMAD_BASE_URL as set by CredentialFiles.
var originSetForKit bool

// selectedBaseURL is the base URL config.Load will use when the environment
// names none, given the --home and --config the flag parser chose: the file it
// reads decides, and one that names none, or that other users can change, means
// the shipped default.
func selectedBaseURL(home, configFlag string) string {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return ShippedBaseURL
	}
	defer restore()
	path, explicit, err := config.ResolveConfigPath(configFlag)
	if err != nil {
		return ShippedBaseURL
	}
	candidates := []string{path}
	if !explicit {
		if legacy, err := config.LegacyConfigPath(); err == nil {
			candidates = append(candidates, legacy)
		}
	}
	for _, path := range candidates {
		opened, err := cliutil.ReadOpenedConfig(path)
		if os.IsNotExist(err) {
			continue
		}
		var file struct {
			BaseURL string `toml:"base_url"`
		}
		if err != nil || opened.PermErr != nil || toml.Unmarshal(opened.Data, &file) != nil || file.BaseURL == "" {
			return ShippedBaseURL
		}
		return file.BaseURL
	}
	return ShippedBaseURL
}

// noValueFlags is every flag of the program that takes no value; any other flag
// takes the next argument. TestStartupFlagsAgreeWithTheParser fails when a reprint
// changes one.
var noValueFlags = map[string]bool{
	"--active":                true,
	"--agent":                 true,
	"--all":                   true,
	"--allow-partial-failure": true,
	"--article-internal":      true,
	"--at-risk":               true,
	"--compact":               true,
	"--confirm":               true,
	"--csv":                   true,
	"--debug-mismatches":      true,
	"--dry-run":               true,
	"--expand":                true,
	"--full":                  true,
	"--high-only":             true,
	"--human-friendly":        true,
	"--idempotent":            true,
	"--ignore-missing":        true,
	"--include-unassigned":    true,
	"--internal":              true,
	"--json":                  true,
	"--latest-only":           true,
	"--launch":                true,
	"--no-cache":              true,
	"--no-color":              true,
	"--no-input":              true,
	"--no-learn":              true,
	"--no-prune":              true,
	"--no-validate":           true,
	"--plain":                 true,
	"--pretty":                true,
	"--promoted":              true,
	"--public":                true,
	"--quiet":                 true,
	"--receipt":               true,
	"--send":                  true,
	"--shared":                true,
	"--ssl-verify":            true,
	"--stdin":                 true,
	"--strict":                true,
	"--tombstones":            true,
	"--warnings":              true,
	"--yes":                   true,
}

// StartupHomeAndConfig is what the flag parser will make of --home and --config.
// The kit starts before the parser can run, and a package-level Declarations may
// not reach the command tree. FlagValues is wider on purpose, to hold every file a
// command line can name; it cannot choose, since "--select --home x" is no home.
func StartupHomeAndConfig(args []string) (home, configFlag string) {
	return parsedValue(args, "--home", credkit.FlagValues(args, "--home")),
		parsedValue(args, "--config", credkit.FlagValues(args, "--config"))
}

// parsedValue picks, from what FlagValues found for flag, the value the parser
// will settle on: the last one not taken as another flag's value.
func parsedValue(args []string, flag string, found []string) string {
	var parsed string
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		if !strings.HasPrefix(name, "-") || name == "-" {
			continue
		}
		if !hasValue {
			if noValueFlags[name] || (len(name) > 2 && name[1] != '-') {
				continue // a switch, or a shorthand with its value attached
			}
			if i++; i < len(args) {
				value = args[i]
			}
		}
		if name == flag {
			parsed = value
		}
	}
	for _, v := range found {
		if v == parsed {
			return v
		}
	}
	return ""
}

// CredentialFiles is Declarations.Files. The kit takes its allowed origin from the
// first file that names a base URL, read or not by this program, unless BaseURL.Env
// is set, so the base URL in use goes there until Start has read it;
// HoldStartupCredentials takes it back out.
func CredentialFiles(args []string) []string {
	if os.Getenv("ZAMMAD_BASE_URL") == "" {
		originSetForKit = os.Setenv("ZAMMAD_BASE_URL", selectedBaseURL(StartupHomeAndConfig(args))) == nil
	}
	tomlFiles, secretFiles := credentialFilePaths(args)
	return append(tomlFiles, secretFiles...)
}

// HoldStartupCredentials holds what the kit cannot read as a declared field (a
// whole-file secret, the run profiles, the credentials inside the feedback
// endpoint's URL; not URLEnv, which the wiring checker does not read yet) and returns the first configuration or credentials file that does not parse;
// the caller prints the error and exits. A broken file is refused, whatever
// the kit could read out of it: nothing can use it (config.Load fails on it).
func HoldStartupCredentials(args []string) error {
	if originSetForKit {
		originSetForKit = false
		_ = os.Unsetenv("ZAMMAD_BASE_URL")
	}
	holdProfileCredentials(args)
	holdBaseURL("minted:feedback-endpoint", os.Getenv("ZAMMAD_FEEDBACK_ENDPOINT"))
	tomlFiles, secretFiles := credentialFilePaths(args)
	for _, path := range secretFiles {
		if opened, err := cliutil.ReadOpenedConfig(path); err == nil {
			credkit.Hold("minted:webhook-secret", strings.TrimSpace(string(opened.Data)))
		}
	}
	for _, path := range tomlFiles {
		opened, err := cliutil.ReadOpenedConfig(path)
		if err != nil {
			continue
		}
		var cfg config.Config
		if toml.Unmarshal(opened.Data, &cfg) != nil {
			return &BrokenCredentialFileError{Path: path}
		}
	}
	return nil
}

// BrokenCredentialFileError names a configuration or credentials file that
// does not parse. Its text never quotes the file.
type BrokenCredentialFileError struct{ Path string }

func (e *BrokenCredentialFileError) Error() string {
	return e.Path + " does not parse as TOML; fix or remove it (nothing runs while it is broken, and its content is not shown)"
}
