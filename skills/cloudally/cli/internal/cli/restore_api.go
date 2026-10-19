// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"cloudally-pp-cli/internal/client"
)

// flexString decodes a JSON string, number, or null into a string so id
// fields that the API emits inconsistently never break decoding.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flexString(v)
		return nil
	}
	*f = flexString(s)
	return nil
}

// flexInt decodes a JSON number or digit string into an int64; anything
// else decodes to zero so one odd row never aborts a whole page.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if fv, ferr := strconv.ParseFloat(s, 64); ferr == nil {
			*f = flexInt(int64(fv))
			return nil
		}
		*f = 0
		return nil
	}
	*f = flexInt(v)
	return nil
}

// taskEntity mirrors one entry of TaskViewWithEntities.entities.
type taskEntity struct {
	ID              flexString         `json:"id"`
	UserID          flexString         `json:"userId"`
	TaskID          flexString         `json:"taskId"`
	Entity          string             `json:"entity"`
	Status          string             `json:"status"`
	LastBackupDates map[string]flexInt `json:"lastBackupDates"`
	FullName        string             `json:"fullName"`
	ExtID           flexString         `json:"extId"`
}

func (e taskEntity) backupDates() map[string]int64 {
	out := make(map[string]int64, len(e.LastBackupDates))
	for k, v := range e.LastBackupDates {
		out[k] = int64(v)
	}
	return out
}

// taskWithEntities mirrors TaskViewWithEntities.
type taskWithEntities struct {
	ID              flexString         `json:"id"`
	Account         flexString         `json:"account"`
	Type            string             `json:"type"`
	Source          string             `json:"source"`
	Status          string             `json:"status"`
	SnapshotDate    flexInt            `json:"snapshotDate"`
	LastBackupDates map[string]flexInt `json:"lastBackupDates"`
	NumOfEntities   int                `json:"numOfEntities"`
	Entities        []taskEntity       `json:"entities"`
}

// taskProgress mirrors TaskView.progress.
type taskProgress struct {
	Current   flexInt `json:"current"`
	State     string  `json:"state"`
	Service   string  `json:"service"`
	Cancelled bool    `json:"cancelled"`
}

func (p *taskProgress) String() string {
	if p == nil {
		return ""
	}
	parts := []string{}
	if p.State != "" {
		parts = append(parts, p.State)
	}
	parts = append(parts, fmt.Sprintf("%d%%", int64(p.Current)))
	if p.Cancelled {
		parts = append(parts, "cancelled")
	}
	return strings.Join(parts, " ")
}

// restoreTaskView mirrors TaskView rows from the RESTORE task list.
type restoreTaskView struct {
	ID           flexString    `json:"id"`
	Account      string        `json:"account"`
	Type         string        `json:"type"`
	Source       string        `json:"source"`
	Status       string        `json:"status"`
	BackupTaskID flexString    `json:"backupTaskId"`
	SnapshotDate flexInt       `json:"snapshotDate"`
	CreateDate   string        `json:"createDate"`
	Progress     *taskProgress `json:"progress"`
	Alias        string        `json:"alias"`
}

type restoreTaskPage struct {
	Page          int               `json:"page"`
	TotalPages    int               `json:"totalPages"`
	Total         int               `json:"total"`
	NextPageToken string            `json:"nextPageToken"`
	Data          []restoreTaskView `json:"data"`
}

// fetchTaskWithEntities loads one backup task and its entities.
func fetchTaskWithEntities(ctx context.Context, c *client.Client, path string) (*taskWithEntities, error) {
	raw, err := c.GetNoCache(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var task taskWithEntities
	if err := json.Unmarshal(raw, &task); err != nil {
		return nil, fmt.Errorf("decoding task response: %w", err)
	}
	return &task, nil
}

// fetchRestoreTasks pages through every RESTORE task visible in scope.
func fetchRestoreTasks(ctx context.Context, c *client.Client, scope restoreScope) ([]restoreTaskView, error) {
	return fetchTasksOfType(ctx, c, scope, restoreTaskListType)
}

// fetchTasksOfType pages through every task of one type visible in scope.
// RESTORE and EXPORT share this shape, so they share the paging.
func fetchTasksOfType(ctx context.Context, c *client.Client, scope restoreScope, taskType string) ([]restoreTaskView, error) {
	var all []restoreTaskView
	seen := map[string]bool{}
	// The first request leaves page unset so the server's own first page
	// applies; later requests ask for the page after the one returned.
	// Dedupe by id guards against either page-numbering convention.
	nextPage := 0
	for fetched := 0; fetched < restoreTaskMaxPages; fetched++ {
		params := map[string]string{
			"type":     taskType,
			"pageSize": strconv.Itoa(restoreTaskPageSize),
		}
		if nextPage > 0 {
			params["page"] = strconv.Itoa(nextPage)
		}
		raw, err := c.GetNoCache(ctx, scope.tasksPath(), params)
		if err != nil {
			return all, err
		}
		var pg restoreTaskPage
		if err := json.Unmarshal(raw, &pg); err != nil {
			var list []restoreTaskView
			if lerr := json.Unmarshal(raw, &list); lerr != nil {
				return all, fmt.Errorf("decoding task list: %w", err)
			}
			pg.Data = list
		}
		fresh := 0
		for _, t := range pg.Data {
			key := string(t.ID)
			if key == "" || !seen[key] {
				seen[key] = true
				all = append(all, t)
				fresh++
			}
		}
		if fresh == 0 || len(pg.Data) < restoreTaskPageSize {
			break
		}
		if pg.Page > 0 {
			nextPage = pg.Page + 1
		} else {
			nextPage = fetched + 2
		}
		if pg.TotalPages > 0 && nextPage > pg.TotalPages {
			break
		}
	}
	return all, nil
}
