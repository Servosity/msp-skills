// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package main

import (
	"go/parser"
	"go/token"
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
	c := got["items.create"]
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
	s := cs["items.create"]
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
