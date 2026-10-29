// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json warranty-mcp-local-writes): over MCP a call
// writes this machine's local data only with confirm: true, on the real binaries.

package credwire

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// homeFiles is every file under dir.
func homeFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, strings.TrimPrefix(p, dir))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A writer without confirm says what it needs, on a fresh home.
func TestMCPWriterAsksForConfirm(t *testing.T) {
	_, mcp := binaries(t)
	s := newSandbox(t)
	v := newVendors(t, s)
	for tool, args := range map[string]map[string]any{"sync": {"resources": "apple"}, "learnings_forget": {"args": "x"}} {
		if frames, _ := mcpCall(t, mcp, allVendorEnv(t, s, v), tool, args); !strings.Contains(frames, "confirm") {
			t.Errorf("%s without confirm did not ask for it: %s", tool, frames)
		}
	}
}

// Both directions: confirm: true still reaches the write.
func TestMCPConfirmStillWrites(t *testing.T) {
	_, mcp := binaries(t)
	s := newSandbox(t)
	v := newVendors(t, s)
	env := allVendorEnv(t, s, v)
	frames, _ := mcpCall(t, mcp, env, "lookup", map[string]any{"serial": "1ABC234", "vendor": "dell", "confirm": true})
	if !strings.Contains(frames, "saved") || !strings.Contains(frames, "true") {
		t.Fatalf("lookup confirm: true did not save: %s", frames)
	}
	found := false
	for _, f := range homeFiles(t, s.home) {
		found = found || strings.HasSuffix(f, "data-warranty.db")
	}
	if !found {
		t.Errorf("lookup confirm: true left no mirror")
	}
	frames, _ = mcpCall(t, mcp, env, "sync", map[string]any{"resources": "apple", "confirm": true})
	if strings.Contains(frames, "confirm: true") {
		t.Errorf("sync confirm: true was still refused: %s", frames)
	}
}

// governance.md says WARRANTY_NO_LOCAL_DATA=1 stops the CLI writing any local data.
func TestNoLocalDataWritesNoFiles(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	env := s.env("WARRANTY_NO_LOCAL_DATA=1")
	for _, args := range [][]string{{"version"}, {"report", "--json"}, {"doctor", "--json"}, {"learnings", "list", "--json"}} {
		run(t, cli, env, args...)
		if files := homeFiles(t, s.home); len(files) > 0 {
			t.Errorf("%v with WARRANTY_NO_LOCAL_DATA=1 wrote %v", args, files)
		}
	}
}

// Every write to an opened file goes through a kit writer: a handle os.OpenFile,
// os.Create or os.CreateTemp returned has no Write or WriteString call, except in
// the declared credential-store exemption. Codex's inputs: the lock file's NUL and
// the journal's repaired line feed.
func TestNoRawFileWrites(t *testing.T) {
	exempt := map[string]bool{filepath.Join("..", "cliutil", "paths.go"): true}
	roots := []string{"..", filepath.Join("..", "..", "cmd")}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "credkit" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || exempt[p] {
				return nil
			}
			fset := token.NewFileSet()
			f, perr := parser.ParseFile(fset, p, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(f, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					return true
				}
				handles := map[string]bool{}
				ast.Inspect(fn.Body, func(m ast.Node) bool {
					switch x := m.(type) {
					case *ast.AssignStmt:
						if len(x.Rhs) == 1 && isOSOpen(x.Rhs[0]) {
							if id, ok := x.Lhs[0].(*ast.Ident); ok {
								handles[id.Name] = true
							}
						}
					case *ast.CallExpr:
						if sel, ok := x.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "Write" || sel.Sel.Name == "WriteString" || sel.Sel.Name == "WriteAt") {
							if id, ok := sel.X.(*ast.Ident); ok && handles[id.Name] {
								t.Errorf("%s: %s.%s writes past the kit", fset.Position(x.Pos()), id.Name, sel.Sel.Name)
							}
						}
					}
					return true
				})
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func isOSOpen(e ast.Expr) bool {
	c, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os" && (sel.Sel.Name == "OpenFile" || sel.Sel.Name == "Create" || sel.Sel.Name == "CreateTemp")
}

// The kit hands a credential in the environment down to the connector's own child.
// Input: any credential in the server's environment, then lookup over MCP. Each
// vendor's credential has to reach that vendor through the child, on its own and
// with the others set.
func TestMirroredToolReachesItsVendorWithEnvCredentials(t *testing.T) {
	_, mcp := binaries(t)
	only := func(env []string, prefix string) []string {
		var out []string
		for _, kv := range env {
			if !strings.HasPrefix(kv, "WARRANTY_DELL_") && !strings.HasPrefix(kv, "WARRANTY_LENOVO_") && !strings.HasPrefix(kv, "WARRANTY_APPLE_") || strings.HasPrefix(kv, prefix) {
				out = append(out, kv)
			}
		}
		return out
	}
	for _, c := range []struct {
		name, prefix, vendor, serial, host, want string
	}{
		{"Dell", "WARRANTY_DELL_", "dell", "1ABC234", hostDell, "Bearer " + stubDellToken},
		{"Lenovo", "WARRANTY_LENOVO_", "lenovo", "PF1ABCDE", hostLenovo, stubLenovoKey},
		{"Apple", "WARRANTY_APPLE_", "apple", "C02XG0FDH7JY", hostAppleAPI, "Bearer " + stubAppleToken},
		{"Dell with the others set", "WARRANTY_", "dell", "1ABC234", hostDell, "Bearer " + stubDellToken},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := newSandbox(t)
			v := newVendors(t, s)
			env := only(allVendorEnv(t, s, v), c.prefix)
			frames, _ := mcpCall(t, mcp, env, "lookup", map[string]any{"serial": c.serial, "vendor": c.vendor})
			if strings.Contains(frames, "withheld") {
				t.Errorf("the child was refused: %s", frames)
			}
			if !v.carried(c.host, c.want) {
				t.Errorf("the credential never reached %s through the child: %s", c.host, frames)
			}
		})
	}
}

// WARRANTY_MCP_READ_ONLY=1 outranks confirm: true: report journals nothing.
func TestConfirmDoesNotLiftReadOnlyMode(t *testing.T) {
	_, mcp := binaries(t)
	s := newSandbox(t)
	v := newVendors(t, s)
	env := append(allVendorEnv(t, s, v), "WARRANTY_MCP_READ_ONLY=1")
	mcpCall(t, mcp, env, "report", map[string]any{"confirm": true})
	if files := homeFiles(t, s.home); len(files) > 0 {
		t.Errorf("report confirm: true under read-only mode wrote %v", files)
	}
}

// A writer over MCP needs confirm: true when the store already exists too, where
// the no-store refusal does not apply.
func TestMCPWriterRefusedOnAnExistingStore(t *testing.T) {
	_, mcp := binaries(t)
	s := newSandbox(t)
	v := newVendors(t, s)
	env := allVendorEnv(t, s, v)
	mcpCall(t, mcp, env, "sync", map[string]any{"resources": "apple", "confirm": true})
	if len(homeFiles(t, s.home)) == 0 {
		t.Fatal("sync confirm: true created nothing to test against")
	}
	frames, _ := mcpCall(t, mcp, env, "sync", map[string]any{"resources": "apple"})
	if !strings.Contains(frames, "nothing was written") {
		t.Errorf("sync without confirm ran on an existing store: %s", frames)
	}
}

// treeState is every file and directory under dir, files with their content hash
// and mtime. SQLite's own -shm and -wal files are left out: a read-only handle can
// leave them, with no data in them.
func treeState(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			out[strings.TrimPrefix(p, dir)+"/"] = "dir"
		}
		return nil
	})
	for _, f := range homeFiles(t, dir) {
		if strings.HasSuffix(f, "-shm") || strings.HasSuffix(f, "-wal") {
			continue
		}
		b, err := os.ReadFile(dir + f)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(dir + f)
		if err != nil {
			t.Fatal(err)
		}
		out[f] = fmt.Sprintf("%x %d", sha256.Sum256(b), info.ModTime().UnixNano())
	}
	return out
}

// everyToolCall is each listed tool with its required arguments and no confirm.
func everyToolCall(t *testing.T, mcp string, env []string) map[string]map[string]any {
	t.Helper()
	frames, _ := mcpRequest(t, mcp, env, "tools/list", map[string]any{})
	calls := map[string]map[string]any{}
	for _, line := range strings.Split(frames, "\n") {
		var msg struct {
			ID     int `json:"id"`
			Result struct {
				Tools []struct {
					Name        string `json:"name"`
					InputSchema struct {
						Required   []string `json:"required"`
						Properties map[string]struct {
							Type string `json:"type"`
						} `json:"properties"`
					} `json:"inputSchema"`
				} `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &msg) != nil || msg.ID != 2 {
			continue
		}
		for _, tool := range msg.Result.Tools {
			args := map[string]any{}
			for _, name := range tool.InputSchema.Required {
				switch tool.InputSchema.Properties[name].Type {
				case "integer", "number":
					args[name] = 1
				case "boolean":
					args[name] = false
				case "array":
					args[name] = []string{"1ABC234"}
				default:
					args[name] = "1ABC234"
				}
			}
			calls[tool.Name] = args
		}
	}
	if len(calls) < 10 {
		t.Fatalf("tools/list gave %d tools: %s", len(calls), frames)
	}
	calls["lookup"] = map[string]any{"serial": "1ABC234", "vendor": "dell"}
	return calls
}

// One fact decides every local write over MCP: confirm: true. Inputs: every listed
// tool, typed or mirrored, on a fresh home and on one that already has a store,
// with and without read-only mode; plus a confirm that is not the boolean true.
func TestMCPNoToolChangesAnythingWithoutConfirm(t *testing.T) {
	_, mcp := binaries(t)
	seed := newSandbox(t)
	seedVendors := newVendors(t, seed)
	calls := everyToolCall(t, mcp, allVendorEnv(t, seed, seedVendors))
	readOnly := []string{"WARRANTY_MCP_READ_ONLY=1"}
	someTools := map[string]map[string]any{}
	for _, tool := range []string{"dell_summary", "playbook_list", "learnings_stats", "report", "lookup"} {
		if _, ok := calls[tool]; !ok {
			t.Fatalf("%s is not listed", tool)
		}
		someTools[tool] = calls[tool]
	}
	withSQL := map[string]map[string]any{"sql": {"query": "SELECT 1"}}
	for k, v := range someTools {
		withSQL[k] = v
	}
	for _, c := range []struct {
		name   string
		seeded bool
		extra  []string
		tools  map[string]map[string]any
		legacy bool
	}{
		{"legacy unscoped store only", false, nil, withSQL, true},
		{"fresh home", false, nil, calls, false},
		{"existing store", true, nil, calls, false},
		{"fresh home, read-only", false, readOnly, someTools, false},
		{"existing store, read-only", true, readOnly, someTools, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t)
			v := newVendors(t, s)
			env := append(allVendorEnv(t, s, v), c.extra...)
			if c.seeded {
				base := allVendorEnv(t, s, v)
				mcpCall(t, mcp, base, "lookup", map[string]any{"serial": "1ABC234", "vendor": "dell", "confirm": true})
				mcpCall(t, mcp, base, "sync", map[string]any{"resources": "apple", "confirm": true})
				mcpCall(t, mcp, base, "teach", map[string]any{"query": "an ordinary query", "resource-type": "domain", "resource": "example.com", "confirm": true})
				// Codex's inputs: an expired response-cache file, and a legacy unscoped
				// store with no owner marker.
				old := time.Now().Add(-time.Hour)
				for _, f := range []string{"cache/http/resources/apple/old.json", "cache/http/resources/dell/old.json", "data/data.db"} {
					p := filepath.Join(s.home, filepath.FromSlash(f))
					if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.Chtimes(p, old, old); err != nil {
						t.Fatal(err)
					}
				}
			}
			if c.legacy {
				p := filepath.Join(s.home, "data", "data.db")
				if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before := treeState(t, s.home)
			if c.seeded && len(before) == 0 {
				t.Fatal("the seed calls created nothing to test against")
			}
			for tool, args := range c.tools {
				frames, _ := mcpCall(t, mcp, env, tool, args)
				// Both directions: a read still answers from what is already there.
				if c.seeded && (tool == "playbook_list" || tool == "learnings_stats" || tool == "report") && strings.Contains(frames, `"isError":true`) {
					t.Errorf("%s on an existing store failed: %s", tool, frames)
				}
				if after := treeState(t, s.home); !reflect.DeepEqual(before, after) {
					for f, h := range after {
						if before[f] != h {
							t.Errorf("%s changed %s", tool, f)
						}
					}
					for f := range before {
						if _, ok := after[f]; !ok {
							t.Errorf("%s removed %s", tool, f)
						}
					}
					before = after
				}
			}
			mcpCall(t, mcp, env, "lookup", map[string]any{"serial": "1ABC234", "vendor": "dell", "confirm": "true"})
			if after := treeState(t, s.home); !reflect.DeepEqual(before, after) {
				t.Errorf("lookup with a string confirm wrote: %v", after)
			}
		})
	}
}

// The Apple client ID and key stay in the environment and are held, so the kit would
// refuse a third-party child that inherits them. Input: Apple's variables set and a
// credential file with an ACL entry, which makes the permission check run dsmemberutil.
func TestMembershipLookupRunsWithAppleVariablesSet(t *testing.T) {
	cli, _ := binaries(t)
	if runtime.GOOS != "darwin" {
		t.Skip("dsmemberutil is macOS only")
	}
	s := newSandbox(t)
	v := newVendors(t, s)
	store := filepath.Join(s.home, "data", "credentials.toml")
	if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store, []byte("token_expiry = \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("chmod", "+a", "user:"+u.Username+" allow read,write", store).CombinedOutput(); err != nil {
		t.Skipf("chmod +a: %v %s", err, out)
	}
	res := run(t, cli, allVendorEnv(t, s, v), "lookup", "1ABC234", "--vendor", "dell", "--no-save", "--json", "--no-learn")
	if out := res.stdout + res.stderr; strings.Contains(out, "refused at") || strings.Contains(out, "withheld") || !strings.Contains(out, `"result"`) {
		t.Errorf("a store the owner's ACL allows was refused: %s", out)
	}
}
