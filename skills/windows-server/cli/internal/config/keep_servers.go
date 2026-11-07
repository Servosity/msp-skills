// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Pinned by handfixes.json (server-profiles).
// The generated save rewrites the config file from the fields it knows, which
// would drop the [[servers]] tables this connector keeps in the same file
// (auth set-credentials and auth logout both save). The rewrite carries them
// over, and refuses to rewrite a file it cannot read whole.

package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

func withServers(path string, data []byte) ([]byte, error) {
	old, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s to keep its [[servers]]: %w", path, err)
	}
	var kept struct {
		Servers []map[string]any `toml:"servers"`
	}
	if err := toml.Unmarshal(old, &kept); err != nil {
		return nil, fmt.Errorf("%s does not parse, so it is not rewritten (its [[servers]] would be lost): fix the file first", path)
	}
	if len(kept.Servers) == 0 {
		return data, nil
	}
	tail, err := toml.Marshal(kept)
	if err != nil {
		return nil, fmt.Errorf("marshaling the [[servers]] of %s: %w", path, err)
	}
	return append(append(bytes.TrimRight(data, "\n"), "\n\n"...), tail...), nil
}
