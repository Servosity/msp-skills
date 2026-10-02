// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package main

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractWireContract(t *testing.T) {
	source := `package cli
 func endpoint() {
 var bodyName string
 var flagPage int
 cmd := &cobra.Command{Annotations: map[string]string{"pp:endpoint":"items.create", "pp:method":"POST", "pp:path":"/items/{id}"}, RunE: func() {
 if !cmd.Flags().Changed("display-name") { return fmt.Errorf("required flag %s not set", "display-name") }
 path = replacePathParam(path, "id", args[0])
 params["page_size"] = formatCLIParamValue(flagPage)
 body["DisplayName"] = bodyName
 }}
 cmd.Flags().StringVar(&bodyName, "display-name", "", "Display name")
 cmd.Flags().IntVar(&flagPage, "page-size", 0, "Page size")
 }
 `
	f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := extractFile(f)
	if err != nil {
		t.Fatal(err)
	}
	c := got["items.create\nPOST /items/{id}"]
	p := c["properties"].(map[string]any)
	if p["DisplayName"].(map[string]any)["type"] != "string" {
		t.Fatal(p)
	}
	if p["page-size"].(map[string]any)["x-wire-name"] != "page_size" {
		t.Fatal(p)
	}
	if p["id"].(map[string]any)["x-location"] != "path" {
		t.Fatal(p)
	}
	required := c["required"].([]string)
	if len(required) != 2 || required[0] != "DisplayName" || required[1] != "id" {
		t.Fatal(required)
	}
}

func TestUnknownBindingFails(t *testing.T) {
	f, _ := parser.ParseFile(token.NewFileSet(), "fixture.go", `package cli; func f(){ _=map[string]string{"pp:endpoint":"items.list", "pp:method":"GET", "pp:path":"/items"}; params["foo"]=unknown }`, 0)
	if _, err := extractFile(f); err == nil {
		t.Fatal("unknown binding must not silently omit contract")
	}
}

func TestNestedRequiredParsedTypesHeadersAndCollision(t *testing.T) {
	source := `package cli
 func endpoint(){
 var bodyName string
 var bodyIDs string
 var flagScopes string
 var bodyID int
 cmd:=&cobra.Command{Annotations:map[string]string{"pp:endpoint":"items.create","pp:method":"POST","pp:path":"/items/{id}"},RunE:func(){
 if !cmd.Flags().Changed("request-name"){return fmt.Errorf("required flag %s not set","request-name")}
 path=replacePathParam(path,"id",args[0])
 headerOverrides["Scopes"]=formatCLIParamValue(flagScopes)
 nestedRequest:=map[string]any{}
 nestedRequest["name"]=bodyName
 parsedIDs,err:=cliutil.ParseStringList(bodyIDs)
 nestedRequest["ids"]=parsedIDs
 bodyMap["requestData"]=nestedRequest
 bodyMap["id"]=bodyID
 }}
 cmd.Flags().StringVar(&bodyName,"request-name","","Name")
 cmd.Flags().StringVar(&bodyIDs,"request-ids","","IDs")
 cmd.Flags().StringVar(&flagScopes,"scopes","","Scopes")
 cmd.Flags().IntVar(&bodyID,"id-2",0,"Body ID")
 }`
	f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := extractFile(f)
	if err != nil {
		t.Fatal(err)
	}
	s := cs["items.create\nPOST /items/{id}"]
	p := s["properties"].(schema)
	nested := p["requestData"].(schema)
	if nested["properties"].(schema)["ids"].(schema)["type"] != "array" {
		t.Fatal(nested)
	}
	if nested["required"].([]string)[0] != "name" {
		t.Fatal(nested)
	}
	if p["scopes"].(schema)["x-location"] != "header" || p["scopes"].(schema)["x-wire-name"] != "Scopes" {
		t.Fatal(p)
	}
	if p["path_id"].(schema)["x-wire-name"] != "id" || p["id"].(schema)["x-location"] != "body" {
		t.Fatal(p)
	}
	if got := s["required"].([]string); len(got) != 2 || got[0] != "path_id" || got[1] != "requestData" {
		t.Fatal(got)
	}
}

func TestArrayAssertionsPreserveElementTypes(t *testing.T) {
	for _, tc := range []struct{ goType, want string }{{"any", ""}, {"interface{}", ""}, {"string", "string"}, {"int64", "integer"}, {"uint32", "integer"}, {"float32", "number"}, {"bool", "boolean"}, {"map[string]any", "object"}} {
		t.Run(tc.goType, func(t *testing.T) {
			source := `package cli; func f(){var bodyIDs string;cmd:=&cobra.Command{Annotations:map[string]string{"pp:endpoint":"items.create","pp:method":"POST","pp:path":"/items"},RunE:func(){var parsed any;json.Unmarshal([]byte(bodyIDs),&parsed);asArray,ok:=parsed.([]` + tc.goType + `);bodyMap["ids"]=asArray}};cmd.Flags().StringVar(&bodyIDs,"ids","","IDs")}`
			f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			cs, err := extractFile(f)
			if err != nil {
				t.Fatal(err)
			}
			items := cs["items.create\nPOST /items"]["properties"].(schema)["ids"].(schema)["items"].(schema)
			if tc.want == "" {
				if len(items) != 0 {
					t.Fatalf("arbitrary JSON elements advertised as %#v", items)
				}
			} else if items["type"] != tc.want {
				t.Fatalf("%s elements advertised as %#v", tc.goType, items)
			}
		})
	}
}

func TestBodyTransportCollisionsInEitherOrder(t *testing.T) {
	for _, loc := range []string{"query", "header", "path"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%v", loc, reverse), func(t *testing.T) {
				binding := `params["value"]=flagValue`
				if loc == "header" {
					binding = `headerOverrides["value"]=flagValue`
				}
				if loc == "path" {
					binding = `path=replacePathParam(path,"value",args[0])`
				}
				body := `bodyMap["value"]=bodyValue`
				statements := binding + `;` + body
				if reverse {
					statements = body + `;` + binding
				}
				source := `package cli;func f(){var flagValue string;var bodyValue int;cmd:=&cobra.Command{Annotations:map[string]string{"pp:endpoint":"items.create","pp:method":"POST","pp:path":"/items/{value}"},RunE:func(){if !cmd.Flags().Changed("value"){return fmt.Errorf("required flag %s not set","value")};` + statements + `}};cmd.Flags().StringVar(&flagValue,"value","","Transport value");cmd.Flags().IntVar(&bodyValue,"body-value",0,"Body value")}`
				f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
				if err != nil {
					t.Fatal(err)
				}
				cs, err := extractFile(f)
				if err != nil {
					t.Fatal(err)
				}
				s := cs["items.create\nPOST /items/{value}"]
				p := s["properties"].(schema)
				alias, ok := p[loc+"_value"].(schema)
				if !ok {
					t.Fatalf("missing transport alias: %#v", p)
				}
				if alias["x-location"] != loc || alias["x-wire-name"] != "value" {
					t.Fatal(alias)
				}
				if p["value"].(schema)["x-location"] != "body" || p["value"].(schema)["type"] != "integer" {
					t.Fatal(p)
				}
				if r := s["required"].([]string); len(r) != 1 || r[0] != loc+"_value" {
					t.Fatalf("required must refer to transport: %#v", r)
				}
			})
		}
	}
}

func TestCatalogArrayHintDoesNotInventGETBody(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		s := object()
		applyArrayBody(s, method, true)
		if _, ok := s["properties"].(schema)["body"]; ok {
			t.Fatalf("%s nil request body advertised as required array: %#v", method, s)
		}
	}
	s := object()
	applyArrayBody(s, "POST", true)
	if s["properties"].(schema)["body"].(schema)["type"] != "array" {
		t.Fatal(s)
	}
}

func TestTypedSliceFlags(t *testing.T) {
	for _, tc := range []struct{ flagType, want string }{{"IntSliceVar", "integer"}, {"Int64SliceVar", "integer"}, {"UintSliceVar", "integer"}, {"Float64SliceVar", "number"}, {"BoolSliceVar", "boolean"}, {"StringArrayVar", "string"}} {
		t.Run(tc.flagType, func(t *testing.T) {
			source := `package cli;func f(){var values []int;cmd:=&cobra.Command{Annotations:map[string]string{"pp:endpoint":"items.create","pp:method":"POST","pp:path":"/items"},RunE:func(){bodyMap["values"]=values}};cmd.Flags().` + tc.flagType + `(&values,"values",nil,"Values")}`
			f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			cs, err := extractFile(f)
			if err != nil {
				t.Fatal(err)
			}
			got := cs["items.create\nPOST /items"]["properties"].(schema)["values"].(schema)
			items, _ := got["items"].(schema)
			if got["type"] != "array" || items["type"] != tc.want {
				t.Fatal(got)
			}
		})
	}
}

func TestRequiredSurvivesJSONAndSupplementMerge(t *testing.T) {
	s := object()
	s["required"] = []any{"tenant"}
	applyArrayBody(s, "POST", true)
	got, _ := s["required"].([]string)
	if fmt.Sprint(got) != "[body tenant]" {
		t.Fatalf("lost JSON required names: %#v", s)
	}
	mergeSchema(s, schema{"required": []any{"revision", "tenant"}})
	if got := fmt.Sprint(s["required"]); got != "[body revision tenant]" {
		t.Fatalf("supplement replaced required list: %s", got)
	}
}

func TestMultipleAnnotatedCommandsFailClosed(t *testing.T) {
	source := `package cli;func f(){first:=&cobra.Command{Annotations:map[string]string{"pp:endpoint":"items.first","pp:method":"GET","pp:path":"/first"}};second:=&cobra.Command{Annotations:map[string]string{"pp:endpoint":"items.second","pp:method":"GET","pp:path":"/second"}}}`
	f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extractFile(f); err == nil {
		t.Fatal("multiple annotated commands silently collapsed into one schema")
	}
}

func runExtractorFixture(t *testing.T, files map[string]string, catalog, supplements string) (map[string]schema, error) {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		path := filepath.Join(root, "skills", "fixture", "cli", "internal", "cli", name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{"skills/fixture/cli/internal/mcp/code_orch.go": catalog, "tools/maintainer/request-contracts/supplements.json": supplements} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "run", "main.go", "-slug", "fixture", "-root", root)
	cmd.Env = append(os.Environ(), "GO111MODULE=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(root, "skills/fixture/cli/internal/mcp/request_contracts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]schema
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got, nil
}

func TestCatalogSameRequestContractsRemainIndependent(t *testing.T) {
	source := `package cli;func f(){_=map[string]string{"pp:endpoint":"items.original","pp:method":"POST","pp:path":"/items"}}`
	catalog := `package mcp;var endpoints=[]endpoint{{ID:"items.first",Method:"POST",Path:"/items",TemplateParams:[]binding{{PublicName:"scope",WireName:"scope"}}},{ID:"items.second",Method:"POST",Path:"/items"}}`
	got, err := runExtractorFixture(t, map[string]string{"items.go": source}, catalog, `{"fixture":{"items.first":{"properties":{"first_only":{"type":"string"}},"required":["first_only"]}}}`)
	if err != nil {
		t.Fatal(err)
	}
	first := got["items.first"]["properties"].(map[string]any)
	second := got["items.second"]["properties"].(map[string]any)
	if first["scope"] == nil || first["first_only"] == nil {
		t.Fatal(first)
	}
	if len(second) != 0 || got["items.second"]["required"] != nil {
		t.Fatalf("catalog metadata leaked to sibling: %#v", got)
	}
}

func TestCrossFileEndpointIdentityFailsClosed(t *testing.T) {
	source := `package cli;func f(){_=map[string]string{"pp:endpoint":"items.list","pp:method":"GET","pp:path":"/items"}}`
	catalog := `package mcp;var endpoints=[]endpoint{{ID:"items.list",Method:"GET",Path:"/items"}}`
	_, err := runExtractorFixture(t, map[string]string{"one.go": source, "two.go": source}, catalog, `{}`)
	if err == nil || !strings.Contains(err.Error(), "duplicate CLI endpoint identity") {
		t.Fatalf("duplicate identity not rejected: %v", err)
	}
}

func TestDuplicateAnnotationIDWithDifferentPathsRemainsResolvable(t *testing.T) {
	source := `package cli;func f(){_=map[string]string{"pp:endpoint":"logs.get","pp:method":"GET","pp:path":"/one"}}`
	second := strings.ReplaceAll(source, "/one", "/two")
	catalog := `package mcp;var endpoints=[]endpoint{{ID:"first.logs.get",Method:"GET",Path:"/one"},{ID:"second.logs.get",Method:"GET",Path:"/two"}}`
	got, err := runExtractorFixture(t, map[string]string{"one.go": source, "two.go": second}, catalog, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatal(got)
	}
}

func TestNativeWireAliasEnrichmentUsesSourcePublicProperty(t *testing.T) {
	dst := object()
	dst["properties"].(schema)["tenant-filter"] = schema{"type": "string", "x-location": "query", "x-wire-name": "tenantFilter"}
	dst["required"] = []string{"tenant-filter"}
	mergeContractSupplement(dst, schema{"required": []any{"tenantFilter"}, "properties": schema{"tenantFilter": schema{"enum": []string{"AllTenants"}}}})
	if got := fmt.Sprint(dst["required"]); got != "[tenant-filter]" {
		t.Fatalf("required alias was not remapped: %s", got)
	}
	p := dst["properties"].(schema)
	if len(p) != 1 || p["tenant-filter"].(schema)["enum"] == nil {
		t.Fatal(p)
	}
}

func TestCatalogWireAliasDoesNotCreateSecondInput(t *testing.T) {
	source := `package cli;func f(){var tenant string;cmd:=&cobra.Command{Annotations:map[string]string{"pp:endpoint":"items.list","pp:method":"GET","pp:path":"/items"},RunE:func(){if !cmd.Flags().Changed("tenant-filter"){return fmt.Errorf("required flag %s not set","tenant-filter")};params["tenantFilter"]=tenant}};cmd.Flags().StringVar(&tenant,"tenant-filter","","Tenant")}`
	catalog := `package mcp;var endpoints=[]endpoint{{ID:"items.list",Method:"GET",Path:"/items",QueryParams:[]binding{{PublicName:"tenantFilter",WireName:"tenantFilter"}}}}`
	got, err := runExtractorFixture(t, map[string]string{"items.go": source}, catalog, `{"fixture":{"items.list":{"properties":{"tenantFilter":{"enum":["AllTenants"]}}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	s := got["items.list"]
	props := s["properties"].(map[string]any)
	if len(props) != 1 || props["tenant-filter"] == nil {
		t.Fatal(props)
	}
	if fmt.Sprint(s["required"]) != "[tenant-filter]" {
		t.Fatal(s)
	}
	if props["tenant-filter"].(map[string]any)["enum"] == nil {
		t.Fatal(props)
	}
}

func TestRequiredOnlySupplementAliasResolvesOrFails(t *testing.T) {
	dst := object()
	dst["properties"].(schema)["tenant-filter"] = schema{"type": "string", "x-location": "query", "x-wire-name": "tenantFilter"}
	mergeContractSupplement(dst, schema{"required": []any{"tenantFilter"}})
	if got := fmt.Sprint(dst["required"]); got != "[tenant-filter]" {
		t.Fatalf("required-only alias not remapped: %s", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("unknown required property must fail closed")
		}
	}()
	mergeContractSupplement(dst, schema{"required": []any{"missing"}})
}

func TestSameFileAnnotationIDWithDifferentPathsResolves(t *testing.T) {
	source := `package cli;func f(){_=map[string]string{"pp:endpoint":"logs.get","pp:method":"GET","pp:path":"/one"}};func g(){_=map[string]string{"pp:endpoint":"logs.get","pp:method":"GET","pp:path":"/two"}}`
	catalog := `package mcp;var endpoints=[]endpoint{{ID:"first.logs.get",Method:"GET",Path:"/one"},{ID:"second.logs.get",Method:"GET",Path:"/two"}}`
	got, err := runExtractorFixture(t, map[string]string{"logs.go": source}, catalog, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatal(got)
	}
}

func TestTemplateBindingsAreRequiredAndDeduplicatePaths(t *testing.T) {
	source := `package cli;func f(){_=map[string]string{"pp:endpoint":"items.get","pp:method":"GET","pp:path":"/items/{id}"};path=replacePathParam(path,"id",args[0])};func g(){_=map[string]string{"pp:endpoint":"tenants.get","pp:method":"GET","pp:path":"/tenants/{tenant}"}}`
	catalog := `package mcp;var endpoints=[]endpoint{{ID:"items.get",Method:"GET",Path:"/items/{id}",TemplateParams:[]binding{{PublicName:"item-id",WireName:"id"}}},{ID:"tenants.get",Method:"GET",Path:"/tenants/{tenant}",TemplateParams:[]binding{{PublicName:"tenant-scope",WireName:"tenant"}}}}`
	got, err := runExtractorFixture(t, map[string]string{"items.go": source}, catalog, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if p := got["items.get"]["properties"].(schema); len(p) != 1 || p["id"] == nil {
		t.Fatalf("duplicate template/path inputs: %#v", p)
	}
	if r := fmt.Sprint(got["tenants.get"]["required"]); r != "[tenant-scope]" {
		t.Fatalf("template input is not required: %s", r)
	}
}
