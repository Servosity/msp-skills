// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// datto-rmm handfix export-follows-next-page-url (tests-helper finding
// TestExportWalksEveryDattoPageAndStops): `export <list>` reads a Datto list
// the way the API pages it, from page 0 along pageDetails.nextPageUrl until
// it is null (the walk sync and --all use: paginatedGet keeps only the next
// link's page value and re-requests the configured host), and writes each
// record. 4.34.2's generic export (resource_paths.go "page" walk) asked
// page=1 first, skipping page 0; found no item array in Datto's
// {"pageDetails":...,"<plural>":[...]} envelope, so it wrote every page
// object as one record; and never saw an empty page (a page past the end is
// an envelope too), so it walked 100000 pages, against the live API, before
// failing. Under --dry-run the walk previews the first request and writes
// nothing (handfix dry-run-changes-nothing). A single-record export
// (`export <resource> <id>`) keeps the generated path; under --dry-run its
// preview goes to stdout, never into --output.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"datto-rmm-pp-cli/internal/credkit"
)

func init() {
	registerNovelCommand(guardDattoExport)
}

// dattoExportPageSize is the most records Datto returns in one page.
const dattoExportPageSize = 250

func guardDattoExport(root *cobra.Command, flags *rootFlags) {
	cmd := findExact(root, []string{"export"})
	if cmd == nil || cmd.RunE == nil || flags == nil {
		return
	}
	generated := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) == 1 {
			if _, err := resourceReadPath(args[0]); err == nil {
				return runDattoExportList(c, flags, args[0])
			}
		}
		if flags.dryRun {
			if f := c.Flags().Lookup("output"); f != nil && f.Value.String() != "" {
				fmt.Fprintf(c.ErrOrStderr(), "--dry-run: the preview goes to stdout; %s is left alone\n", f.Value.String())
				_ = f.Value.Set("")
			}
		}
		return generated(c, args)
	}
}

func runDattoExportList(cmd *cobra.Command, flags *rootFlags, resource string) (err error) {
	format, _ := cmd.Flags().GetString("format")
	outputFile, _ := cmd.Flags().GetString("output")
	limit, _ := cmd.Flags().GetInt("limit")
	noCache, _ := cmd.Flags().GetBool("no-cache")
	path, err := resourceReadPath(resource)
	if err != nil {
		return usageErr(err)
	}
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	if noCache {
		c.NoCache = true
	}
	sizeParam := "max"
	if resource == "activity-logs" {
		sizeParam = "size"
	}
	size, fetchAll := dattoExportPageSize, true
	if limit > 0 && limit <= dattoExportPageSize {
		size, fetchAll = limit, false
	}
	data, err := paginatedGet(cmd.Context(), c, path, map[string]string{sizeParam: strconv.Itoa(size)}, nil, fetchAll,
		"page", "cursor", sizeParam, size, "pageDetails.nextPageUrl", "")
	if err != nil {
		return classifyAPIError(cmd.OutOrStdout(), err, flags)
	}
	if isDryRunResponseForClient(c, data) {
		return nil // the client printed the first request; nothing is written
	}
	if responseDeclaresFailure(data) { // datto-rmm handfix: inband-error-field-is-a-failure (--limit reads one page without the walk)
		return fmt.Errorf("export %s: the response reports a failure in its body; nothing was written", resource)
	}
	items, err := dattoExportItems(data, path)
	if err != nil {
		return fmt.Errorf("export %s: %w", resource, err)
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	var out io.Writer = cmd.OutOrStdout()
	var outFile *os.File
	var outKit *credkit.Writer
	if outputFile != "" {
		// The file opens only once every page is read, through the credential
		// kit (handfixes.json credkit-wiring), and is closed on every return.
		f, ferr := openExportOutput(outputFile)
		if ferr != nil {
			return ferr
		}
		outFile = f
		defer func() {
			if outFile != nil {
				_ = outFile.Close()
			}
		}()
		outKit = credkit.NewWriter(f, credkit.SinkFile)
		out = outKit
	}
	// Whole records, and one Write for the document (handfix
	// export-hands-the-kit-whole-records).
	if format == "jsonl" {
		records := &exportRecords{w: out}
		for _, item := range items {
			if err := records.add(item); err != nil {
				return fmt.Errorf("writing export: %w", err)
			}
		}
		if err := records.flush(); err != nil {
			return fmt.Errorf("writing export: %w", err)
		}
	} else {
		var doc bytes.Buffer
		enc := json.NewEncoder(&doc)
		enc.SetIndent("", "  ")
		if err := enc.Encode(items); err != nil {
			return err
		}
		if _, err := out.Write(doc.Bytes()); err != nil {
			return fmt.Errorf("writing export: %w", err)
		}
	}
	if outFile != nil {
		outFile = nil
		if err := outKit.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			return fmt.Errorf("closing export file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Exported %d records to %s\n", len(items), outputFile)
	}
	return nil
}

// dattoExportItems is the record array of a Datto list answer: the array the
// walk returned, or the collection inside its envelope.
func dattoExportItems(data json.RawMessage, path string) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if json.Unmarshal(data, &items) == nil {
		return items, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("the response is neither a list nor an object")
	}
	field, preserve := paginatedCollectionEnvelopeField(obj, path)
	if nested, ok := extractPaginatedItems(obj, path); ok {
		return nested, nil
	}
	if preserve && json.Unmarshal(obj[field], &items) == nil {
		return items, nil
	}
	return nil, fmt.Errorf("the response holds no record list")
}
