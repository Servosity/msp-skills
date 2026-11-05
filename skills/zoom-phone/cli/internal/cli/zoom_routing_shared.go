// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared wire types and helpers for the routing trace, routing audit and
// callbacks novel commands. Field names follow
// spec/zoom-phone-openapi.json exactly; nothing here is guessed.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
)

const (
	routingHoursBusiness = "business_hours"
	routingHoursClosed   = "closed_hours"

	routingTypeAutoReceptionist = "autoReceptionist"
	routingTypeCallQueue        = "callQueue"
	routingTypeUser             = "user"
	routingTypeCommonArea       = "commonArea"
	routingTypeSharedLineGroup  = "sharedLineGroup"

	routingDefaultMaxPages = 20
)

// routingFlexInt decodes an integer that may arrive as a JSON number or a
// numeric string. Absent or unparseable values decode to 0 (no Zoom IVR
// action uses 0).
type routingFlexInt int

func (n *routingFlexInt) UnmarshalJSON(b []byte) error {
	t := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if t == "" || t == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.Atoi(t)
	if err != nil {
		f, ferr := strconv.ParseFloat(t, 64)
		if ferr != nil {
			*n = 0
			return nil
		}
		v = int(f)
	}
	*n = routingFlexInt(v)
	return nil
}

type routingNumberRef struct {
	ID     string `json:"id"`
	Number string `json:"number"`
}

type routingNamedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type routingHoliday struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// GET /phone/auto_receptionists -> auto_receptionists[]
type routingAutoReceptionist struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	ExtensionID     string             `json:"extension_id"`
	ExtensionNumber zoomFlexString     `json:"extension_number"`
	PhoneNumbers    []routingNumberRef `json:"phone_numbers"`
	// Pointer so an absent key (unknown) differs from an empty list (none).
	HolidayHours *[]routingHoliday `json:"holiday_hours"`
	Site         routingNamedRef   `json:"site"`
}

// GET /phone/call_queues -> call_queues[]
type routingCallQueue struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	ExtensionID     string             `json:"extension_id"`
	ExtensionNumber zoomFlexString     `json:"extension_number"`
	PhoneNumbers    []routingNumberRef `json:"phone_numbers"`
	Status          string             `json:"status"`
	Site            routingNamedRef    `json:"site"`
}

// GET /phone/users -> users[]
type routingUser struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Email            string             `json:"email"`
	ExtensionID      string             `json:"extension_id"`
	ExtensionNumber  zoomFlexString     `json:"extension_number"`
	Status           string             `json:"status"`
	ActivationStatus string             `json:"activation_status"`
	PhoneNumbers     []routingNumberRef `json:"phone_numbers"`
}

// GET /phone/common_areas -> common_areas[]
type routingCommonArea struct {
	ID              string             `json:"id"`
	DisplayName     string             `json:"display_name"`
	ExtensionNumber zoomFlexString     `json:"extension_number"`
	PhoneNumbers    []routingNumberRef `json:"phone_numbers"`
}

// GET /phone/shared_line_groups -> shared_line_groups[]
type routingSharedLineGroup struct {
	ID              string             `json:"id"`
	DisplayName     string             `json:"display_name"`
	ExtensionID     string             `json:"extension_id"`
	ExtensionNumber zoomFlexString     `json:"extension_number"`
	PhoneNumbers    []routingNumberRef `json:"phone_numbers"`
	Status          string             `json:"status"`
}

// GET /phone/sites -> sites[]
type routingSite struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	MainAutoReceptionist struct {
		ID              string         `json:"id"`
		ExtensionID     string         `json:"extension_id"`
		ExtensionNumber zoomFlexString `json:"extension_number"`
		Name            string         `json:"name"`
	} `json:"main_auto_receptionist"`
}

// GET /phone/call_queues/{callQueueId}/members -> call_queue_members[]
type routingQueueMember struct {
	ID          string `json:"id"`
	Level       string `json:"level"`
	Name        string `json:"name"`
	ReceiveCall *bool  `json:"receive_call"`
	ExtensionID string `json:"extension_id"`
}

// GET /phone/auto_receptionists/{autoReceptionistId}/ivr
type routingIVR struct {
	KeyActions []routingKeyAction `json:"key_actions"`
	NoAction   *routingNoAction   `json:"caller_enters_no_action"`
}

type routingKeyAction struct {
	Action routingFlexInt   `json:"action"`
	Key    string           `json:"key"`
	Target routingIVRTarget `json:"target"`
}

type routingNoAction struct {
	Action    routingFlexInt   `json:"action"`
	ForwardTo routingIVRTarget `json:"forward_to"`
}

type routingIVRTarget struct {
	DisplayName     string         `json:"display_name"`
	ExtensionID     string         `json:"extension_id"`
	ExtensionNumber zoomFlexString `json:"extension_number"`
	ID              string         `json:"id"`
	PhoneNumber     string         `json:"phone_number"`
}

// routingIVRAction describes one IVR action code from the key_actions /
// caller_enters_no_action enums in the spec.
type routingIVRAction struct {
	nodeType string
	label    string
	// targetType is the entity family the target must belong to, or "" when
	// the action has no account entity to verify (external numbers, menus).
	targetType string
	skip       bool
}

func routingIVRActionInfo(code int, noInput bool) routingIVRAction {
	switch code {
	case -1:
		if noInput {
			return routingIVRAction{nodeType: "disconnect", label: "disconnect"}
		}
		return routingIVRAction{skip: true}
	case 2:
		return routingIVRAction{nodeType: routingTypeUser, label: "forward_to_user", targetType: routingTypeUser}
	case 3:
		return routingIVRAction{nodeType: "zoomRoom", label: "forward_to_zoom_room"}
	case 4:
		return routingIVRAction{nodeType: routingTypeCommonArea, label: "forward_to_common_area", targetType: routingTypeCommonArea}
	case 5:
		return routingIVRAction{nodeType: "ciscoPolycomRoom", label: "forward_to_cisco_polycom_room"}
	case 6:
		return routingIVRAction{nodeType: routingTypeAutoReceptionist, label: "forward_to_auto_receptionist", targetType: routingTypeAutoReceptionist}
	case 7:
		return routingIVRAction{nodeType: routingTypeCallQueue, label: "forward_to_call_queue", targetType: routingTypeCallQueue}
	case 8:
		return routingIVRAction{nodeType: routingTypeSharedLineGroup, label: "forward_to_shared_line_group", targetType: routingTypeSharedLineGroup}
	case 9:
		return routingIVRAction{nodeType: "externalContact", label: "forward_to_external_contact"}
	case 10:
		return routingIVRAction{nodeType: "phoneNumber", label: "forward_to_phone_number"}
	case 12:
		return routingIVRAction{nodeType: "message", label: "play_message_then_disconnect"}
	case 15:
		return routingIVRAction{nodeType: "partnerContactCenter", label: "forward_to_partner_contact_center"}
	case 16:
		return routingIVRAction{nodeType: "meetingService", label: "forward_to_meeting_service"}
	case 17:
		return routingIVRAction{nodeType: "meetingServiceNumber", label: "forward_to_meeting_service_number"}
	case 18:
		return routingIVRAction{nodeType: "teams", label: "forward_to_teams"}
	case 19:
		return routingIVRAction{nodeType: "zoomContactCenter", label: "forward_to_zoom_contact_center"}
	case 21:
		return routingIVRAction{nodeType: "menu", label: "repeat_menu_greeting"}
	case 22:
		return routingIVRAction{nodeType: "menu", label: "return_to_root_menu"}
	case 23:
		return routingIVRAction{nodeType: "menu", label: "return_to_previous_menu"}
	case 24:
		return routingIVRAction{nodeType: "zvaReceptionist", label: "forward_to_zva_receptionist"}
	case 25:
		return routingIVRAction{nodeType: "thirdPartyAgent", label: "forward_to_third_party_agent"}
	case 26:
		return routingIVRAction{nodeType: "dialByName", label: "dial_by_name"}
	case 27:
		return routingIVRAction{nodeType: "menu", label: "play_message_then_repeat_menu"}
	case 28:
		return routingIVRAction{nodeType: "menu", label: "play_message_then_root_menu"}
	case 29:
		return routingIVRAction{nodeType: "menu", label: "play_message_then_previous_menu"}
	case 100:
		return routingIVRAction{nodeType: "voicemail", label: "leave_voicemail_current_extension"}
	case 200:
		return routingIVRAction{nodeType: "voicemail", label: "leave_voicemail_user", targetType: routingTypeUser}
	case 300:
		return routingIVRAction{nodeType: "voicemail", label: "leave_voicemail_auto_receptionist", targetType: routingTypeAutoReceptionist}
	case 400:
		return routingIVRAction{nodeType: "voicemail", label: "leave_voicemail_call_queue", targetType: routingTypeCallQueue}
	case 500:
		return routingIVRAction{nodeType: "voicemail", label: "leave_voicemail_shared_line_group", targetType: routingTypeSharedLineGroup}
	case 0:
		return routingIVRAction{skip: true}
	}
	return routingIVRAction{nodeType: "unknown", label: "action_" + strconv.Itoa(code)}
}

// routingPageResult is one paginated list walk.
type routingPageResult struct {
	Items     []json.RawMessage
	Pages     int
	Truncated bool
}

// routingListAll walks a Zoom list endpoint with page_size + next_page_token.
// stop, when non-nil, ends the walk early once it returns true for an item.
// On error the items fetched so far are returned with the error.
func routingListAll(ctx context.Context, c *client.Client, path, itemsKey string, params map[string]string, pageSize, maxPages int, stop func(json.RawMessage) bool) (routingPageResult, error) {
	var res routingPageResult
	if maxPages < 1 {
		maxPages = 1
	}
	token := ""
	// Every cursor used in this walk: a page that hands back one of them
	// (the same token, or an A->B->A cycle) would loop forever or end early
	// looking complete, so it is reported as an error instead.
	seen := map[string]bool{}
	for {
		p := make(map[string]string, len(params)+2)
		for k, v := range params {
			p[k] = v
		}
		if pageSize > 0 {
			p["page_size"] = strconv.Itoa(pageSize)
		}
		if token != "" {
			p["next_page_token"] = token
		}
		raw, err := c.Get(ctx, path, p)
		if err != nil {
			return res, err
		}
		var env map[string]json.RawMessage
		if err := json.Unmarshal(raw, &env); err != nil {
			return res, fmt.Errorf("decode %s: %w", path, err)
		}
		res.Pages++
		var items []json.RawMessage
		if v, ok := env[itemsKey]; ok && len(v) > 0 && string(v) != "null" {
			if err := json.Unmarshal(v, &items); err != nil {
				return res, fmt.Errorf("decode %s.%s: %w", path, itemsKey, err)
			}
		}
		for _, it := range items {
			res.Items = append(res.Items, it)
			if stop != nil && stop(it) {
				return res, nil
			}
		}
		next := ""
		if v, ok := env["next_page_token"]; ok {
			_ = json.Unmarshal(v, &next)
		}
		if next == "" {
			return res, nil
		}
		if seen[next] {
			return res, routingCursorLoopError(path, res.Pages)
		}
		if res.Pages >= maxPages {
			res.Truncated = true
			return res, nil
		}
		seen[next] = true
		token = next
	}
}

// routingCursorLoopError reports a paginated walk whose next_page_token
// repeated a cursor already used in the same walk.
func routingCursorLoopError(path string, pages int) error {
	return fmt.Errorf("%s: next_page_token repeated a cursor already used after %d page(s) (pagination loop); the list is incomplete", path, pages)
}

// routingEffectiveMaxPages applies the dogfood cap (1 page) to a --max-pages value.
func routingEffectiveMaxPages(v int) int {
	if cliutil.IsDogfoodEnv() {
		return 1
	}
	return v
}

// routingErrText renders an API error compactly for per-node / per-fetch
// error fields without dumping a whole response body.
func routingErrText(err error) string {
	if err == nil {
		return ""
	}
	var apiError *client.APIError
	if errors.As(err, &apiError) {
		body := strings.TrimSpace(apiError.Body)
		if len(body) > 160 {
			body = body[:160] + "..."
		}
		if body != "" {
			return fmt.Sprintf("HTTP %d: %s", apiError.StatusCode, body)
		}
		return fmt.Sprintf("HTTP %d", apiError.StatusCode)
	}
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	return msg
}

func routingIsNotFound(err error) bool {
	var ce *cliError
	return errors.As(err, &ce) && ce.code == 3
}

func routingJoinErr(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

func routingCacheKey(path string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(path)
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(params[k])
	}
	return b.String()
}
