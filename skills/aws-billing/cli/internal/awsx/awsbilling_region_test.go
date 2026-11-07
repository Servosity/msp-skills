// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsx

// r1b-inventory-region-resolved: ResolveRegion is the region New's client
// scans, however the region is named, with no request sent.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestR1bResolveRegionIsTheLiveClientsRegion(t *testing.T) {
	isolateAWSCredentialSources(t)
	cfgFile := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(cfgFile, []byte("[default]\nregion = eu-central-1\n\n[profile west]\nregion = us-west-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, flag, profile, awsRegion, defRegion, config, want string
	}{
		{"flag", "ap-south-1", "", "us-east-2", "", "", "ap-south-1"},
		{"AWS_REGION", "", "", "us-east-2", "ca-central-1", "", "us-east-2"},
		{"AWS_DEFAULT_REGION", "", "", "", "ca-central-1", "", "ca-central-1"},
		{"the default profile's region", "", "", "", "", cfgFile, "eu-central-1"},
		{"a named profile's region", "", "west", "", "", cfgFile, "us-west-2"},
		{"nothing names one", "", "", "", "", "", "us-east-1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("AWS_REGION", c.awsRegion)
			t.Setenv("AWS_DEFAULT_REGION", c.defRegion)
			if c.config != "" {
				t.Setenv("AWS_CONFIG_FILE", c.config)
			}
			got, err := ResolveRegion(context.Background(), c.profile, c.flag)
			if err != nil {
				t.Fatal(err)
			}
			client, err := New(context.Background(), c.profile, c.flag)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want || client.Region != c.want {
				t.Errorf("ResolveRegion = %q, New's client region = %q, want %q", got, client.Region, c.want)
			}
		})
	}
}
