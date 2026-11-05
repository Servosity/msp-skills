// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	"acme-pp-cli/internal/config"
	"acme-pp-cli/internal/credkit"
)

// logger captures os.Stderr before main runs and writes after Start: routed.
var logger = log.New(os.Stderr, "acme: ", 0)

func init() { sql.Register("credkit-sqlite", credkit.SQLDriver(fakeDriver{})) }

func env(name string) string { return os.Getenv(name) }

func Execute() int {
	cfg, _ := config.Load()
	_ = cfg
	_ = env("ACME_DEBUG")
	if v, ok := os.LookupEnv("ACME_TOKEN"); ok {
		_ = v
	}
	client := &http.Client{Transport: credkit.Transport(http.DefaultTransport)}
	req, _ := http.NewRequest("GET", "https://auth.example.com", nil)
	_, _ = client.Do(req)
	_ = credkit.WriteFile("out.json", nil, 0o600)
	w, _ := credkit.OpenFile("log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	_ = w
	f, _ := os.OpenFile("in.json", os.O_RDONLY, 0)
	_ = f
	cmd := exec.Command("true")
	cmd.Env = []string{}
	_ = credkit.RunCommand(cmd)
	db, _ := sql.Open("credkit-sqlite", "acme.db")
	_ = db
	fmt.Println("hello")
	logger.Println("done")
	return 0
}

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return nil, nil }
