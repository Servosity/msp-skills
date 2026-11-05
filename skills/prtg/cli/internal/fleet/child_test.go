// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// childEnv makes this test binary the child a Runner starts, on every OS: with
// "args" it prints the arguments it was given on one line, with "env" its
// environment one variable per line. Not a PRTG_ name, so Scrub passes it.
const childEnv = "FLEET_TEST_CHILD"

func TestMain(m *testing.M) {
	switch os.Getenv(childEnv) {
	case "args":
		fmt.Println(strings.Join(os.Args[1:], " "))
		os.Exit(0)
	case "env":
		for _, kv := range os.Environ() {
			fmt.Println(kv)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// testChild is a Runner whose child is this test binary printing what mode
// asks for.
func testChild(t *testing.T, mode string, insecure bool) Runner {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(childEnv, mode)
	return Runner{Exe: exe, Insecure: insecure}
}
