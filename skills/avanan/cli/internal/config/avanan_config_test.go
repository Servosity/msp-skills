// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guard for handfixes.json entry
// "credkit-wiring" (class secret-echo).

package config

import (
	"avanan-pp-cli/internal/credkit"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

// TestHoldCredentialsHoldsEveryConfigStringField sets every exported string
// field and string map value of Config, then holds it: every one not named in
// notCredentials must be held, so a credential field a reprint adds is held
// without anyone listing it, and every named setting must not be.
func TestHoldCredentialsHoldsEveryConfigStringField(t *testing.T) {
	c := &Config{}
	v := reflect.ValueOf(c).Elem()
	want := map[string]string{}
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if !field.IsExported() {
			continue
		}
		value := fmt.Sprintf("hc%02dx", i)
		switch fv := v.Field(i); fv.Kind() {
		case reflect.String:
			fv.SetString(value)
			want[field.Name] = value
		case reflect.Map:
			if fv.Type().Key().Kind() == reflect.String && fv.Type().Elem().Kind() == reflect.String {
				fv.Set(reflect.ValueOf(map[string]string{"X-Any": value}))
				want[field.Name] = value
			}
		}
	}
	if _, ok := want["Headers"]; !ok {
		t.Fatal("Config.Headers is not a string map any more; update HoldCredentials")
	}
	HoldCredentials(c)
	for name, value := range want {
		held := configHeldNow(t, value)
		if notCredentials[name] && held {
			t.Errorf("setting %s was held as a credential", name)
		}
		if !notCredentials[name] && !held {
			t.Errorf("credential field %s was not held", name)
		}
	}
}

// configHeldNow reports whether the kit withholds v from a file write.
func configHeldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte("x "+v+" x"), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("probe write: %v", err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}
