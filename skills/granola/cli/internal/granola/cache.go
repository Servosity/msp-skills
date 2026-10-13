// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package granola provides the typed domain model for Granola.ai meeting data
// and a TipTap renderer. On Granola v7.4x+ the encrypted desktop cache
// (cache-v6.json.enc) and internal api.granola.ai endpoints are sealed
// (app-private behind Granola's macOS Keychain access group), so this
// connector sources ENTIRELY from the public REST API — the CLI populates the
// local SQLite store from REST and reconstructs these types from that store.
// The types below are the compatibility shape the read commands consume.
package granola

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Document is a Granola meeting note. The notes field is left as
// json.RawMessage so callers can either decode it themselves or hand it
// straight to the TipTap renderer in tiptap.go.
type Document struct {
	ID                    string               `json:"id"`
	CreatedAt             string               `json:"created_at,omitempty"`
	UpdatedAt             string               `json:"updated_at,omitempty"`
	DeletedAt             *string              `json:"deleted_at,omitempty"`
	Title                 string               `json:"title,omitempty"`
	Notes                 json.RawMessage      `json:"notes,omitempty"`
	NotesPlain            string               `json:"notes_plain,omitempty"`
	NotesMarkdown         string               `json:"notes_markdown,omitempty"`
	Transcribe            bool                 `json:"transcribe,omitempty"`
	Type                  string               `json:"type,omitempty"`
	ValidMeeting          bool                 `json:"valid_meeting,omitempty"`
	WorkspaceID           string               `json:"workspace_id,omitempty"`
	LastIndexedAt         string               `json:"last_indexed_at,omitempty"`
	HasShareableLink      bool                 `json:"has_shareable_link,omitempty"`
	SharingLinkVisibility string               `json:"sharing_link_visibility,omitempty"`
	CreationSource        string               `json:"creation_source,omitempty"`
	PrivacyModeEnabled    bool                 `json:"privacy_mode_enabled,omitempty"`
	GoogleCalendarEvent   *GoogleCalendarEvent `json:"google_calendar_event,omitempty"`
	People                *DocPeople           `json:"people,omitempty"`
	UserID                string               `json:"user_id,omitempty"`
	Summary               json.RawMessage      `json:"summary,omitempty"`
	Overview              json.RawMessage      `json:"overview,omitempty"`
	SelectedTemplate      json.RawMessage      `json:"selected_template,omitempty"`
}

// DocPeople carries the people block from a document. Granola stores both
// the attendee list (who was invited) and creator (who recorded).
type DocPeople struct {
	Attendees []DocPerson `json:"attendees,omitempty"`
	Creator   *DocPerson  `json:"creator,omitempty"`
}

// DocPerson is the lightweight person record inside documents.people.
type DocPerson struct {
	Name           string `json:"name,omitempty"`
	Email          string `json:"email,omitempty"`
	ResponseStatus string `json:"response_status,omitempty"`
	Avatar         string `json:"avatar,omitempty"`
}

// TranscriptSegment matches the cache shape under state.transcripts[doc_id][].
type TranscriptSegment struct {
	ID         string `json:"id,omitempty"`
	DocumentID string `json:"document_id,omitempty"`
	// Source is the capture channel. LIVE grammar (public REST API
	// speaker.source, and what the local store holds): "microphone" (near
	// end) | "speaker" (far end). The older "system"/"mixed" values came from
	// the pre-7.4x desktop cache, which Granola made app-private -- a
	// third-party binary can no longer read it, so those values should not
	// appear. See classifySource in internal/cli/preflight.go: an
	// unrecognized label is surfaced, never silently counted as zero.
	Source            string  `json:"source,omitempty"`
	Text              string  `json:"text,omitempty"`
	StartTimestamp    string  `json:"start_timestamp,omitempty"` // ISO-8601
	EndTimestamp      string  `json:"end_timestamp,omitempty"`
	Confidence        float64 `json:"confidence,omitempty"`
	IsFinal           bool    `json:"is_final,omitempty"`
	TranscriberUserID *string `json:"transcriber_user_id,omitempty"`
}

// MeetingMetadata is keyed by document_id in state.meetingsMetadata.
type MeetingMetadata struct {
	Creator      *CalendarInvitee  `json:"creator,omitempty"`
	Attendees    []CalendarInvitee `json:"attendees,omitempty"`
	Conferencing json.RawMessage   `json:"conferencing,omitempty"`
	URL          string            `json:"url,omitempty"`
}

// CalendarInvitee is one attendee on the calendar event.
type CalendarInvitee struct {
	Name           string `json:"name,omitempty"`
	Email          string `json:"email,omitempty"`
	ResponseStatus string `json:"response_status,omitempty"`
}

// GoogleCalendarEvent is the raw calendar event attached to a doc.
type GoogleCalendarEvent struct {
	ID          string            `json:"id,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	Description string            `json:"description,omitempty"`
	Location    string            `json:"location,omitempty"`
	Start       json.RawMessage   `json:"start,omitempty"`
	End         json.RawMessage   `json:"end,omitempty"`
	Attendees   []CalendarInvitee `json:"attendees,omitempty"`
	HtmlLink    string            `json:"htmlLink,omitempty"`
}

// DocumentListMetadata is the metadata for a Granola folder (also called a
// "documentList" in the cache).
type DocumentListMetadata struct {
	ID                   string          `json:"id"`
	Title                string          `json:"title,omitempty"`
	Description          string          `json:"description,omitempty"`
	ParentDocumentListID string          `json:"parent_document_list_id,omitempty"`
	WorkspaceID          string          `json:"workspace_id,omitempty"`
	UserRole             string          `json:"user_role,omitempty"`
	Preset               string          `json:"preset,omitempty"`
	CreatedAt            string          `json:"created_at,omitempty"`
	UpdatedAt            string          `json:"updated_at,omitempty"`
	DeletedAt            *string         `json:"deleted_at,omitempty"`
	Visibility           string          `json:"visibility,omitempty"`
	Members              []DocPerson     `json:"members,omitempty"`
	Rules                json.RawMessage `json:"rules,omitempty"`
	IsFavourited         bool            `json:"is_favourited,omitempty"`
}

// ListRule is one rule in the listRules engine. The cache stores them as
// either an array of rules or an empty list per folder.
type ListRule struct {
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Field    string          `json:"field,omitempty"`
	Operator string          `json:"operator,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"`
	ListID   string          `json:"list_id,omitempty"`
}

// PanelTemplate is an AI panel template ("recipe-like" structured prompt).
type PanelTemplate struct {
	ID          string          `json:"id,omitempty"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Category    string          `json:"category,omitempty"`
	Slug        string          `json:"slug,omitempty"`
	IsGranola   bool            `json:"is_granola,omitempty"`
	Sections    json.RawMessage `json:"sections,omitempty"`
}

// Recipe is one entry in publicRecipes / userRecipes / sharedRecipes. The
// cache shapes the description+instructions under config{}, which we
// flatten on the typed surface via the Description/Instructions accessors.
type Recipe struct {
	ID            string          `json:"id,omitempty"`
	Slug          string          `json:"slug,omitempty"`
	UserID        string          `json:"user_id,omitempty"`
	WorkspaceID   string          `json:"workspace_id,omitempty"`
	Visibility    string          `json:"visibility,omitempty"`
	Config        RecipeConfig    `json:"config,omitempty"`
	CreatedAt     string          `json:"created_at,omitempty"`
	UpdatedAt     string          `json:"updated_at,omitempty"`
	DeletedAt     *string         `json:"deleted_at,omitempty"`
	PublisherSlug string          `json:"publisher_slug,omitempty"`
	CreatorInfo   json.RawMessage `json:"creator_info,omitempty"`
	// Source is a compatibility field ("public", "user", or "shared") carried
	// on the typed recipe surface. Left unset on the REST-fed path.
	Source string `json:"-"`
	// Name is derived from slug when the inner Config has no name. Most
	// publicRecipes use slug as the canonical identifier.
	Name string `json:"-"`
	// Category is filled from the parent panelTemplate if linked, otherwise
	// from the recipe's allowed_views.
	Category string `json:"-"`
}

// RecipeConfig is the inner config block on a Recipe.
type RecipeConfig struct {
	Description        string          `json:"description,omitempty"`
	Instructions       string          `json:"instructions,omitempty"`
	Examples           json.RawMessage `json:"examples,omitempty"`
	AllowedViews       []string        `json:"allowed_views,omitempty"`
	AllowedRecipeViews []string        `json:"allowed_recipe_views,omitempty"`
	GenerateArtifact   bool            `json:"generate_artifact,omitempty"`
	ShowInSharedTabs   bool            `json:"show_in_shared_tabs,omitempty"`
}

// RecipeUsage is one entry under state.recipesUsage keyed by recipe_id.
type RecipeUsage struct {
	RecipeID   string `json:"recipe_id,omitempty"`
	TotalCount string `json:"total_count,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

// ChatThread is one entry under state.entities.chat_thread.
type ChatThread struct {
	ID          string         `json:"id,omitempty"`
	Type        string         `json:"type,omitempty"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	UpdatedAt   string         `json:"updated_at,omitempty"`
	DeletedAt   *string        `json:"deleted_at,omitempty"`
	Data        ChatThreadData `json:"data,omitempty"`
}

// ChatThreadData is the inner data block on a ChatThread.
type ChatThreadData struct {
	UserID      string `json:"user_id,omitempty"`
	Title       string `json:"title,omitempty"`
	GroupingKey string `json:"grouping_key,omitempty"`
	DocumentID  string `json:"document_id,omitempty"`
}

// ChatMessage is one entry under state.entities.chat_message.
type ChatMessage struct {
	ID          string          `json:"id,omitempty"`
	Type        string          `json:"type,omitempty"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
	CreatedAt   string          `json:"created_at,omitempty"`
	UpdatedAt   string          `json:"updated_at,omitempty"`
	Data        ChatMessageData `json:"data,omitempty"`
}

// ChatMessageData is the inner data block on a ChatMessage.
type ChatMessageData struct {
	ThreadID  string `json:"thread_id,omitempty"`
	Role      string `json:"role,omitempty"` // user | assistant | system
	TurnIndex int    `json:"turn_index,omitempty"`
	RawText   string `json:"raw_text,omitempty"`
}

// Workspace is one workspace under state.workspaceData.workspaces[].
type Workspace struct {
	PlanType  string          `json:"plan_type,omitempty"`
	Role      string          `json:"role,omitempty"`
	Workspace json.RawMessage `json:"workspace,omitempty"`
}

// Cache is the parsed cache-v6.json file.
type Cache struct {
	Version               int
	Documents             map[string]Document
	Transcripts           map[string][]TranscriptSegment
	MeetingsMetadata      map[string]MeetingMetadata
	DocumentLists         map[string][]string // list_id -> [meeting_ids]
	DocumentListsMetadata map[string]DocumentListMetadata
	ListRules             map[string][]ListRule
	PanelTemplates        []PanelTemplate
	PublicRecipes         []Recipe
	UserRecipes           []Recipe
	SharedRecipes         []Recipe
	RecipesUsage          map[string]RecipeUsage
	ChatThreads           map[string]ChatThread
	ChatMessages          map[string]ChatMessage
	Workspaces            []Workspace
	rawState              map[string]json.RawMessage
}

// DocumentByID returns the document with the given id, or nil.
func (c *Cache) DocumentByID(id string) *Document {
	if c == nil {
		return nil
	}
	d, ok := c.Documents[id]
	if !ok {
		return nil
	}
	return &d
}

// TranscriptByID returns the cached transcript segments for the given doc id.
func (c *Cache) TranscriptByID(id string) []TranscriptSegment {
	if c == nil {
		return nil
	}
	return c.Transcripts[id]
}

// MeetingMetadataByID returns the metadata for the given doc id, or nil.
func (c *Cache) MeetingMetadataByID(id string) *MeetingMetadata {
	if c == nil {
		return nil
	}
	m, ok := c.MeetingsMetadata[id]
	if !ok {
		return nil
	}
	return &m
}

// FolderByName returns the documentListMetadata whose title equals name
// (case-insensitive) or whose id equals name, or nil.
func (c *Cache) FolderByName(name string) *DocumentListMetadata {
	if c == nil {
		return nil
	}
	lname := strings.ToLower(name)
	for _, dl := range c.DocumentListsMetadata {
		if dl.ID == name || strings.EqualFold(dl.Title, lname) || strings.ToLower(dl.Title) == lname {
			res := dl
			return &res
		}
	}
	return nil
}

// FolderMeetings returns the meeting IDs that belong to the named folder,
// resolving via DocumentLists (the direct membership map). Auto-rule
// membership is not re-evaluated here; the cache's DocumentLists block
// reflects Granola's already-applied rules.
func (c *Cache) FolderMeetings(folderID string) []string {
	if c == nil {
		return nil
	}
	out := append([]string(nil), c.DocumentLists[folderID]...)
	return out
}

// SortedDocumentIDs returns doc IDs sorted by descending created_at.
func (c *Cache) SortedDocumentIDs() []string {
	if c == nil {
		return nil
	}
	ids := make([]string, 0, len(c.Documents))
	for id := range c.Documents {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return c.Documents[ids[i]].CreatedAt > c.Documents[ids[j]].CreatedAt
	})
	return ids
}

// RecipesAll returns the merged list of recipes (public + user + shared).
func (c *Cache) RecipesAll() []Recipe {
	if c == nil {
		return nil
	}
	out := make([]Recipe, 0, len(c.PublicRecipes)+len(c.UserRecipes)+len(c.SharedRecipes))
	out = append(out, c.PublicRecipes...)
	out = append(out, c.UserRecipes...)
	out = append(out, c.SharedRecipes...)
	return out
}

// RawState exposes the raw per-key state JSON for debugging / extras.
func (c *Cache) RawState() map[string]json.RawMessage {
	return c.rawState
}

// ParseISO is a small helper for parsing Granola's ISO-8601 timestamps.
func ParseISO(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp %q", s)
}
