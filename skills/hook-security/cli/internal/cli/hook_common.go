// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hook-security-pp-cli/internal/client"
)

const hookDateLayout = "2006-01-02"

type hookPerson struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type hookCourse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type hookEnrollment struct {
	ID               string      `json:"id"`
	OrganizationID   string      `json:"organization_id"`
	OrganizationName string      `json:"organization_name"`
	Course           hookCourse  `json:"course"`
	Person           *hookPerson `json:"person"`
	Status           string      `json:"status"`
	Overdue          bool        `json:"overdue"`
	AssignedAt       *string     `json:"assigned_at"`
	DueAt            *string     `json:"due_at"`
}

type hookSimulations struct {
	Sent               int      `json:"sent"`
	Opened             int      `json:"opened"`
	Clicked            int      `json:"clicked"`
	Landed             int      `json:"landed"`
	Engaged            int      `json:"engaged"`
	Reported           *int     `json:"reported,omitempty"`
	ClickRate          *float64 `json:"click_rate"`
	EngagementRate     *float64 `json:"engagement_rate"`
	EngagementMeasured bool     `json:"engagement_measured"`
}

type hookTraining struct {
	Assigned       int      `json:"assigned"`
	Completed      int      `json:"completed"`
	Outstanding    int      `json:"outstanding"`
	Overdue        int      `json:"overdue"`
	CompletionRate *float64 `json:"completion_rate"`
}

// hookScan walks a cursor-paginated list endpoint for at most maxPages pages.
// truncated is true when more pages remained, so callers can say the answer
// is partial instead of presenting it as complete.
func hookScan[T any](ctx context.Context, c *client.Client, path string, params map[string]string, limit, maxPages int) (rows []T, truncated bool, err error) {
	query := map[string]string{"limit": strconv.Itoa(limit)}
	for k, v := range params {
		query[k] = v
	}
	for page := 0; page < maxPages; page++ {
		raw, err := c.GetNoCache(ctx, path, query)
		if err != nil {
			return rows, false, err
		}
		var body struct {
			Data       []T     `json:"data"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return rows, false, fmt.Errorf("decoding %s: %w", path, err)
		}
		rows = append(rows, body.Data...)
		if body.NextCursor == nil || *body.NextCursor == "" {
			return rows, false, nil
		}
		query["cursor"] = *body.NextCursor
	}
	return rows, true, nil
}

// hookCtx bounds the whole command only when --timeout was set explicitly.
// Each request already has the default timeout, and --max-scan-pages bounds
// the scan, so the 60-second default must not cut off a large MSP mid-walk.
func hookCtx(cmd *cobra.Command, flags *rootFlags) (context.Context, context.CancelFunc) {
	if flags.timeoutExplicit {
		return boundCtx(cmd.Context(), flags)
	}
	return cmd.Context(), func() {}
}

// hookError adds the one recovery step Hook's coverage_too_large error needs.
func hookError(w io.Writer, err error, flags *rootFlags) error {
	var apiError *client.APIError
	if errors.As(err, &apiError) && strings.Contains(apiError.Body, "coverage_too_large") {
		classified := apiErr(fmt.Errorf("%w\nhint: this key covers too many organizations for one request; scope the command with --org <organization id>", err))
		writeAPIErrorEnvelope(w, flags, classified, ExitCode(classified))
		return classified
	}
	return classifyAPIError(w, err, flags)
}

func hookCheckDate(flag, value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(hookDateLayout, value); err != nil {
		return usageErr(fmt.Errorf("--%s must be YYYY-MM-DD, got %q", flag, value))
	}
	return nil
}

func hookRate(rate *float64) string {
	if rate == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", *rate*100)
}

func hookString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
