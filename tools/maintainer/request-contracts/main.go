// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// request-contracts extracts request schemas from the checked-in CLI Go AST.
// Run from the repository root: go run ./tools/maintainer/request-contracts/main.go -slug hudu
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type schema = map[string]any
type flagInfo struct {
	name, typ, itemType, description string
	required                         bool
}

func lit(e ast.Expr) string {
	if x, ok := e.(*ast.BasicLit); ok && x.Kind == token.STRING {
		v, _ := strconv.Unquote(x.Value)
		return v
	}
	return ""
}
func ident(e ast.Expr) string {
	if x, ok := e.(*ast.Ident); ok {
		return x.Name
	}
	return ""
}
func callName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}
func object() schema {
	return schema{"type": "object", "properties": schema{}, "additionalProperties": true}
}
func inspect(n ast.Node, f func(ast.Node)) {
	ast.Inspect(n, func(n ast.Node) bool {
		if n != nil {
			f(n)
		}
		return true
	})
}
func extractFile(file *ast.File) (map[string]schema, error) {
	result := map[string]schema{}
	for _, d := range file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		meta := map[string]string{}
		annotationCount := 0
		inspect(fn.Body, func(n ast.Node) {
			if kv, ok := n.(*ast.KeyValueExpr); ok && strings.HasPrefix(lit(kv.Key), "pp:") {
				meta[lit(kv.Key)] = lit(kv.Value)
				if lit(kv.Key) == "pp:endpoint" {
					annotationCount++
				}
			}
		})
		if annotationCount > 1 {
			return nil, fmt.Errorf("%s contains multiple annotated commands", fn.Name.Name)
		}
		id := meta["pp:endpoint"]
		if id == "" {
			continue
		}
		flags := map[string]*flagInfo{}
		requiredFlags := map[string]bool{}
		vars := map[string]string{}
		inspect(fn.Body, func(n ast.Node) {
			if vs, ok := n.(*ast.ValueSpec); ok {
				for _, name := range vs.Names {
					vars[name.Name] = ident(vs.Type)
				}
			}
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return
			}
			name := callName(c.Fun)
			if strings.HasSuffix(name, "Var") || strings.HasSuffix(name, "VarP") {
				if len(c.Args) < 4 {
					return
				}
				u, ok := c.Args[0].(*ast.UnaryExpr)
				if !ok {
					return
				}
				v := ident(u.X)
				if v == "" {
					return
				}
				typ := "string"
				switch {
				case strings.HasPrefix(name, "Bool"):
					typ = "boolean"
				case strings.HasPrefix(name, "Int"), strings.HasPrefix(name, "Uint"):
					typ = "integer"
				case strings.HasPrefix(name, "Float"):
					typ = "number"
				}
				itemType := ""
				if strings.Contains(name, "Slice") || strings.Contains(name, "Array") {
					itemType = typ
					typ = "array"
				}
				flags[v] = &flagInfo{itemType: itemType, name: lit(c.Args[1]), typ: typ, description: lit(c.Args[len(c.Args)-1])}
			}
			if name == "Errorf" && len(c.Args) > 1 && strings.Contains(lit(c.Args[0]), "required flag") {
				requiredFlags[lit(c.Args[1])] = true
			}
			if name == "MarkFlagRequired" && len(c.Args) > 0 {
				requiredFlags[lit(c.Args[0])] = true
			}
		})
		for _, f := range flags {
			f.required = requiredFlags[f.name]
		}
		out := object()
		out["x-source-method"] = meta["pp:method"]
		out["x-source-path"] = meta["pp:path"]
		props := out["properties"].(schema)
		required := map[string]bool{}
		var problems []string
		values := map[*ast.Object]schema{}
		origins := map[*ast.Object]*flagInfo{}
		var exprSchema func(ast.Expr) schema
		exprSchema = func(e ast.Expr) schema {
			if i, ok := e.(*ast.Ident); ok && i.Obj != nil {
				if s, exists := values[i.Obj]; exists {
					return s
				}
			}
			if v := ident(e); v != "" {
				if f := flags[v]; f != nil {
					s := schema{"type": f.typ, "description": f.description}
					if f.typ == "array" {
						s["items"] = schema{"type": f.itemType}
					}
					return s
				}
				switch vars[v] {
				case "int", "int64":
					return schema{"type": "integer"}
				case "bool":
					return schema{"type": "boolean"}
				case "float64":
					return schema{"type": "number"}
				case "string":
					return schema{"type": "string"}
				}
				if v == "true" || v == "false" {
					return schema{"type": "boolean"}
				}
				return schema{}
			}
			switch x := e.(type) {
			case *ast.IndexExpr:
				if ident(x.X) == "args" {
					return schema{"type": "string"}
				}
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					return schema{"type": "string"}
				}
				return schema{"type": "number"}
			case *ast.CompositeLit:
				if _, ok := x.Type.(*ast.MapType); ok {
					s := object()
					p := s["properties"].(schema)
					for _, el := range x.Elts {
						if kv, ok := el.(*ast.KeyValueExpr); ok {
							p[lit(kv.Key)] = exprSchema(kv.Value)
						}
					}
					return s
				}
				if _, ok := x.Type.(*ast.ArrayType); ok {
					return declaredTypeSchema(x.Type)
				}
			case *ast.TypeAssertExpr:
				return declaredTypeSchema(x.Type)
			case *ast.CallExpr:
				switch callName(x.Fun) {
				case "ParseBool":
					return schema{"type": "boolean"}
				case "ParseInt", "Atoi":
					return schema{"type": "integer"}
				case "ParseFloat":
					return schema{"type": "number"}
				case "ParseStringList":
					return schema{"type": "array", "items": schema{"type": "string"}}
				case "ParseIntList":
					return schema{"type": "array", "items": schema{"type": "integer"}}
				}
				if len(x.Args) > 0 {
					return exprSchema(x.Args[0])
				}
			}
			return schema{}
		}
		// Find the flag behind formatting/parsing expressions.
		underlying := func(e ast.Expr) *flagInfo {
			var f *flagInfo
			inspect(e, func(n ast.Node) {
				if i, ok := n.(*ast.Ident); ok && flags[i.Name] != nil {
					f = flags[i.Name]
				} else if i, ok := n.(*ast.Ident); ok && origins[i.Obj] != nil {
					f = origins[i.Obj]
				}
			})
			return f
		}
		add := func(wire, loc string, e ast.Expr) {
			f := underlying(e)
			name := wire
			s := exprSchema(e)
			if f != nil && loc != "body" {
				name = f.name
			}
			if len(s) == 0 && f == nil {
				problems = append(problems, loc+":"+wire)
			}
			needed := (f != nil && f.required) || s["required"] != nil
			if idx, ok := e.(*ast.IndexExpr); ok && ident(idx.X) == "args" {
				needed = true
			}
			putBinding(props, required, name, wire, loc, s, needed)
		}
		inspect(fn.Body, func(n ast.Node) {
			if c, ok := n.(*ast.CallExpr); ok && callName(c.Fun) == "Unmarshal" && len(c.Args) == 2 {
				if u, ok := c.Args[1].(*ast.UnaryExpr); ok {
					if dest, ok := u.X.(*ast.Ident); ok && dest.Obj != nil {
						origins[dest.Obj] = underlying(c.Args[0])
						values[dest.Obj] = schema{"description": "JSON value"}
					}
				}
			}
			if a, ok := n.(*ast.AssignStmt); ok {
				if len(a.Rhs) > 0 {
					for j, lhs := range a.Lhs {
						if dest, ok := lhs.(*ast.Ident); ok && dest.Obj != nil {
							rhs := a.Rhs[0]
							if j < len(a.Rhs) {
								rhs = a.Rhs[j]
							}
							values[dest.Obj] = exprSchema(rhs)
							origins[dest.Obj] = underlying(rhs)
						}
					}
				}
				for j, lhs := range a.Lhs {
					idx, ok := lhs.(*ast.IndexExpr)
					if !ok || j >= len(a.Rhs) {
						continue
					}
					wire := lit(idx.Index)
					if wire == "" {
						continue
					}
					switch ident(idx.X) {
					case "body", "bodyMap":
						add(wire, "body", a.Rhs[j])
					case "params":
						add(wire, "query", a.Rhs[j])
					case "headers", "requestHeaders", "headerOverrides":
						add(wire, "header", a.Rhs[j])
					default:
						if strings.HasPrefix(ident(idx.X), "nested") {
							if dest, ok := idx.X.(*ast.Ident); ok {
								s := values[dest.Obj]
								if s == nil {
									s = object()
									values[dest.Obj] = s
								}
								if p, ok := s["properties"].(schema); ok {
									child := exprSchema(a.Rhs[j])
									p[wire] = child
									if f := underlying(a.Rhs[j]); (f != nil && f.required) || child["required"] != nil {
										req, _ := s["required"].([]string)
										s["required"] = append(req, wire)
									}
								}
							}
						}
					}
				}
			}
			if c, ok := n.(*ast.CallExpr); ok && callName(c.Fun) == "replacePathParam" && len(c.Args) >= 3 {
				wire := lit(c.Args[1])
				if wire != "" {
					putBinding(props, required, wire, wire, "path", schema{"type": "string"}, true)
				}
			}
		})
		// CLI stdin handling proves an arbitrary JSON body is accepted, but does not
		// justify inventing nested vendor fields or enum values.
		if len(problems) > 0 {
			return nil, fmt.Errorf("%s has unsupported bindings: %s", id, strings.Join(problems, ", "))
		}
		if len(required) > 0 {
			r := []string{}
			for k := range required {
				r = append(r, k)
			}
			sort.Strings(r)
			out["required"] = r
		}
		if _, exists := result[id]; exists {
			return nil, fmt.Errorf("duplicate endpoint %s", id)
		}
		result[id] = out
	}
	return result, nil
}

func main() {
	slug := flag.String("slug", "", "Connector slug")
	root := flag.String("root", ".", "Repository root")
	output := flag.String("output", "", "Output file (defaults to connector request_contracts.json)")
	flag.Parse()
	if *slug == "" {
		panic("-slug is required")
	}
	dir := filepath.Join(*root, "skills", *slug, "cli", "internal", "cli")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		panic(err)
	}
	all := map[string]schema{}
	byRequest := map[string][]schema{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			panic(err)
		}
		contracts, err := extractFile(f)
		if err != nil {
			panic(fmt.Errorf("%s: %w", path, err))
		}
		for id, c := range contracts {
			key := fmt.Sprint(c["x-source-method"]) + " " + fmt.Sprint(c["x-source-path"])
			identity := id + "\n" + key
			if _, exists := all[identity]; exists {
				panic("duplicate CLI endpoint identity: " + identity)
			}
			byRequest[key] = append(byRequest[key], c)
			all[identity] = c
		}
	}
	// The orchestration registry is the coverage authority. It also retains root
	// body shape and promoted template parameters that do not exist as CLI flags.
	catalog := filepath.Join(*root, "skills", *slug, "cli", "internal", "mcp", "code_orch.go")
	f, err := parser.ParseFile(token.NewFileSet(), catalog, nil, 0)
	if err != nil {
		panic(err)
	}
	supplements := map[string]schema{}
	supplementPath := filepath.Join(*root, "tools", "maintainer", "request-contracts", "supplements.json")
	if data, err := os.ReadFile(supplementPath); err == nil {
		var fleet map[string]map[string]schema
		if err := json.Unmarshal(data, &fleet); err != nil {
			panic(err)
		}
		supplements = fleet[*slug]
	} else {
		panic(err)
	}
	seen := map[string]bool{}
	contracts := map[string]schema{}
	inspect(f, func(n ast.Node) {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return
		}
		var id string
		kvmap := map[string]ast.Expr{}
		for _, el := range cl.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				kvmap[ident(kv.Key)] = kv.Value
			}
		}
		id = lit(kvmap["ID"])
		if id == "" {
			return
		}
		key := lit(kvmap["Method"]) + " " + lit(kvmap["Path"])
		candidates := byRequest[key]
		if len(candidates) == 0 {
			supp := supplements[id]
			if supp != nil && supp["x-source-method"] == lit(kvmap["Method"]) && supp["x-source-path"] == lit(kvmap["Path"]) {
				data, _ := json.Marshal(supp)
				copy := schema{}
				if err := json.Unmarshal(data, &copy); err != nil {
					panic(err)
				}
				candidates = []schema{copy}
			} else {
				panic("catalog endpoint missing CLI source: " + id + " (" + key + ")")
			}
		}
		s := candidates[0]
		matchedID := false
		if candidate, ok := all[id+"\n"+key]; ok {
			s = candidate
			matchedID = true
		}
		// Different generated identifiers sometimes refer to the same operation.
		// Match exact method/path and require equivalent schemas for ambiguity.
		for _, c := range candidates {
			if matchedID {
				break
			}
			a, _ := json.Marshal(c)
			b, _ := json.Marshal(s)
			if string(a) != string(b) {
				panic("ambiguous CLI request contract: " + key)
			}
		}
		s = cloneSchema(s)
		contracts[id] = s
		seen[id] = true
		props := s["properties"].(schema)
		applyArrayBody(s, lit(kvmap["Method"]), ident(kvmap["BodyIsArray"]) == "true")
		catalogRequired := map[string]bool{}
		for _, name := range requiredNames(s["required"]) {
			catalogRequired[name] = true
		}
		for _, pair := range []struct{ field, loc string }{{"TemplateParams", "template"}, {"QueryParams", "query"}} {
			if bindings, ok := kvmap[pair.field].(*ast.CompositeLit); ok {
				for _, el := range bindings.Elts {
					if bind, ok := el.(*ast.CompositeLit); ok {
						var public, wire string
						for _, v := range bind.Elts {
							if kv, ok := v.(*ast.KeyValueExpr); ok {
								switch ident(kv.Key) {
								case "PublicName":
									public = lit(kv.Value)
								case "WireName":
									wire = lit(kv.Value)
								}
							}
						}
						if public != "" {
							existing := false
							for name, value := range props {
								p, ok := value.(schema)
								if !ok || p["x-location"] != pair.loc {
									continue
								}
								bound, _ := p["x-wire-name"].(string)
								if bound == "" {
									bound = name
								}
								if bound == wire {
									existing = true
									break
								}
							}
							if !existing {
								if _, present := props[public]; !present {
									putBinding(props, catalogRequired, public, wire, pair.loc, schema{"type": "string", "x-wire-name": wire}, false)
								}
							}
						}
					}
				}
			}
		}
		if len(catalogRequired) > 0 {
			names := []string{}
			for name := range catalogRequired {
				names = append(names, name)
			}
			sort.Strings(names)
			s["required"] = names
		}
	})
	for id, s := range contracts {
		if supplement := supplements[id]; supplement != nil {
			mergeContractSupplement(s, supplement)
		}
		cleanDescriptions(s)
		delete(s, "x-source-method")
		delete(s, "x-source-path")
		delete(s, "x-evidence")
	}
	if len(contracts) == 0 {
		panic("no endpoints found")
	}
	data, err := json.MarshalIndent(contracts, "", "  ")
	if err != nil {
		panic(err)
	}
	if *output == "" {
		*output = filepath.Join(*root, "skills", *slug, "cli", "internal", "mcp", "request_contracts.json")
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("%s: %d endpoint contracts\n", *slug, len(contracts))
}

func mergeSchema(dst, src schema) {
	for k, v := range src {
		if k == "x-evidence" {
			continue
		}
		if k == "required" {
			names := append(requiredNames(dst[k]), requiredNames(v)...)
			dst[k] = requiredNames(names)
			continue
		}
		sm, ok := v.(map[string]any)
		if ok {
			if dm, ok := dst[k].(map[string]any); ok {
				mergeSchema(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}
func cleanDescriptions(s schema) {
	for k, v := range s {
		if k == "description" {
			if str, ok := v.(string); ok {
				s[k] = strings.ReplaceAll(str, "\u2014", " - ")
			}
		}
		if child, ok := v.(map[string]any); ok {
			cleanDescriptions(child)
		}
	}
}

func applyArrayBody(s schema, method string, array bool) {
	if !array || method == "GET" || method == "HEAD" {
		return
	}
	props := s["properties"].(schema)
	props["body"] = schema{"type": "array", "items": schema{}, "x-location": "body", "x-raw-body": true}
	r := requiredNames(s["required"])
	present := false
	for _, name := range r {
		if name == "body" {
			present = true
		}
	}
	if !present {
		r = append(r, "body")
	}
	sort.Strings(r)
	s["required"] = r
}

// declaredTypeSchema never guesses an element type for []any or []interface{}.
func declaredTypeSchema(e ast.Expr) schema {
	switch x := e.(type) {
	case *ast.Ident:
		switch x.Name {
		case "string":
			return schema{"type": "string"}
		case "bool":
			return schema{"type": "boolean"}
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte", "rune":
			return schema{"type": "integer"}
		case "float32", "float64":
			return schema{"type": "number"}
		}
	case *ast.ArrayType:
		return schema{"type": "array", "items": declaredTypeSchema(x.Elt)}
	case *ast.MapType:
		s := object()
		if item := declaredTypeSchema(x.Value); len(item) > 0 {
			s["additionalProperties"] = item
		}
		return s
	case *ast.StarExpr:
		return declaredTypeSchema(x.X)
	}
	return schema{}
}

// Body names are exact wire keys. Other locations get explicit aliases when
// they collide, independent of the source assignment order.
func putBinding(props schema, required map[string]bool, name, wire, loc string, input schema, needed bool) {
	s := schema{}
	for k, v := range input {
		s[k] = v
	}
	s["x-location"] = loc
	if name != wire {
		s["x-wire-name"] = wire
	}
	if previous, ok := props[name].(schema); ok && previous["x-location"] != loc {
		oldLoc, _ := previous["x-location"].(string)
		if oldLoc != "body" {
			alias := oldLoc + "_" + name
			if _, exists := props[alias]; exists {
				panic("request parameter alias collision: " + alias)
			}
			copy := schema{}
			for k, v := range previous {
				copy[k] = v
			}
			if _, ok := copy["x-wire-name"]; !ok {
				copy["x-wire-name"] = name
			}
			props[alias] = copy
			if required[name] {
				required[alias] = true
				delete(required, name)
			}
			delete(props, name)
		}
		if loc != "body" {
			name = loc + "_" + name
			s["x-wire-name"] = wire
			if _, exists := props[name]; exists {
				panic("request parameter alias collision: " + name)
			}
		}
	}
	props[name] = s
	if needed {
		required[name] = true
	}
}

func requiredNames(value any) []string {
	names := map[string]bool{}
	switch v := value.(type) {
	case []string:
		for _, name := range v {
			names[name] = true
		}
	case []any:
		for _, item := range v {
			name, ok := item.(string)
			if !ok {
				panic("required contains non-string value")
			}
			names[name] = true
		}
	case nil:
	default:
		panic("required must be an array")
	}
	result := []string{}
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func cloneSchema(s schema) schema {
	data, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	copy := schema{}
	if err := json.Unmarshal(data, &copy); err != nil {
		panic(err)
	}
	return copy
}

// Native supplements may name an old catalog wire alias. Merge its evidence
// into the source-public property, without reintroducing a duplicate input.
func mergeContractSupplement(dst, src schema) {
	patch := cloneSchema(src)
	renamed := map[string]string{}
	if properties, ok := patch["properties"].(schema); ok {
		target := dst["properties"].(schema)
		for name, value := range properties {
			if _, exists := target[name]; exists {
				continue
			}
			canonical := ""
			for key, entry := range target {
				p, ok := entry.(schema)
				if !ok || p["x-wire-name"] != name {
					continue
				}
				if canonical != "" {
					panic("ambiguous supplement property alias: " + name)
				}
				canonical = key
			}
			if canonical != "" {
				renamed[name] = canonical
				delete(properties, name)
				if previous, ok := properties[canonical].(schema); ok {
					mergeSchema(previous, value.(schema))
				} else {
					properties[canonical] = value
				}
			}
		}
	}
	if value, present := patch["required"]; present {
		names := requiredNames(value)
		for i, name := range names {
			if canonical, ok := renamed[name]; ok {
				names[i] = canonical
			}
		}
		patch["required"] = requiredNames(names)
	}
	mergeSchema(dst, patch)
}
