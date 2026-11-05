// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: follow one inbound number or extension through auto
// receptionists, IVR keys and call queues to where it rings.
// pp:data-source live
// Every hop is a live GET; nothing is read from the local store.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/client"
)

// routingTraceMaxNodes bounds the tree so a dense IVR graph cannot fan out
// exponentially before the depth cap is reached.
const routingTraceMaxNodes = 500

type routingNode struct {
	Type            string         `json:"type"`
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	ExtensionNumber string         `json:"extension_number"`
	ExtensionID     string         `json:"extension_id,omitempty"`
	PhoneNumber     string         `json:"phone_number,omitempty"`
	Key             string         `json:"key,omitempty"`
	Action          string         `json:"action"`
	Children        []*routingNode `json:"children"`
	Cycle           bool           `json:"cycle"`
	// Truncated marks a node whose subtree is incomplete; TruncatedReason is
	// max_depth, node_budget, member_list_max_pages or extension_index_max_pages.
	Truncated       bool   `json:"truncated,omitempty"`
	TruncatedReason string `json:"truncated_reason,omitempty"`
	// noFollow stops expansion of a target that is dead or ambiguous.
	noFollow bool
	Error    string `json:"error,omitempty"`
}

type routingResolved struct {
	Type            string `json:"type"`
	ID              string `json:"id"`
	Name            string `json:"name"`
	ExtensionNumber string `json:"extension_number"`
	PhoneNumber     string `json:"phone_number,omitempty"`
}

type routingTraceResult struct {
	Input    string          `json:"input"`
	Resolved routingResolved `json:"resolved"`
	MaxDepth int             `json:"max_depth"`
	MaxPages int             `json:"max_pages"`
	// Truncated is true when any node in either tree is truncated.
	Truncated bool         `json:"truncated"`
	Business  *routingNode `json:"business"`
	Closed    *routingNode `json:"closed"`
	Note      string       `json:"note,omitempty"`
}

// call_handling/settings responses (auto receptionist routing_action and
// call queue overflow_action share the *_setting union shape).
type routingExtRef struct {
	ExtensionID     string         `json:"extension_id"`
	ExtensionNumber zoomFlexString `json:"extension_number"`
	ExtensionName   string         `json:"extension_name"`
	ExtensionType   string         `json:"extension_type"`
}

type routingHandlingSetting struct {
	LeaveVoicemailTo         *routingExtRef `json:"leave_voicemail_to"`
	ForwardToExtension       *routingExtRef `json:"forward_to_extension"`
	ForwardPhoneNumber       string         `json:"forward_phone_number"`
	ForwardToExternalContact *struct {
		ExternalContactID   string   `json:"external_contact_id"`
		ExternalContactName string   `json:"external_contact_name"`
		PhoneNumbers        []string `json:"phone_numbers"`
	} `json:"forward_to_external_contact"`
	ForwardToPartnerContactCenter *struct {
		PartnerContactCenterID   string `json:"partner_contact_center_id"`
		PartnerContactCenterName string `json:"partner_contact_center_name"`
	} `json:"forward_to_partner_contact_center"`
	// Call queue shape carries the Zoom Contact Center fields flat; the auto
	// receptionist shape nests them under forward_to_zoom_contact_center.
	ZoomContactCenterNumber    string `json:"zoom_contact_center_number"`
	ZoomContactCenterFlowName  string `json:"zoom_contact_center_flow_name"`
	ForwardToZoomContactCenter *struct {
		ZoomContactCenterNumber   string `json:"zoom_contact_center_number"`
		ZoomContactCenterFlowName string `json:"zoom_contact_center_flow_name"`
	} `json:"forward_to_zoom_contact_center"`
	ForwardToTeams *struct {
		TeamsAppID        string `json:"teams_app_id"`
		TeamsVoiceAppName string `json:"teams_voice_app_name"`
	} `json:"forward_to_teams"`
}

type routingARHandling struct {
	CallHandlingSetting struct {
		RoutingAction        string                 `json:"routing_action"`
		RoutingActionSetting routingHandlingSetting `json:"routing_action_setting"`
	} `json:"call_handling_setting"`
}

type routingCQHandling struct {
	CallHandlingSetting struct {
		OverflowAction        string                 `json:"overflow_action"`
		OverflowActionSetting routingHandlingSetting `json:"overflow_action_setting"`
	} `json:"call_handling_setting"`
}

type routingCachedGet struct {
	raw json.RawMessage
	err error
}

type routingTracer struct {
	ctx      context.Context
	c        *client.Client
	maxDepth int
	maxPages int
	nodes    int
	cache    map[string]routingCachedGet

	// indexes holds the auto receptionist and call queue lists, loaded
	// lazily per type, used to resolve and verify routing targets.
	indexes map[string]*routingTraceIndex
}

type routingTraceIndex struct {
	ix        routingEntityIndex
	complete  bool // every page read without error
	truncated bool // --max-pages stopped the walk
	err       error
}

func newNovelRoutingTraceCmd(flags *rootFlags) *cobra.Command {
	var flagNumber, flagExt string
	var flagMaxDepth, flagMaxPages int

	cmd := &cobra.Command{
		Use:   "trace [number-or-extension]",
		Short: "Follow one inbound number through auto receptionists, IVR keys and call queues to where it rings",
		Long: strings.TrimSpace(`
Use this command to follow one inbound phone number (or extension) through auto receptionists, IVR keys and call queues to where it rings, per business and closed hours. Do NOT use this command for an account-wide sweep of broken routing; use 'routing audit' instead. Do NOT use it to see what a user holds; use 'footprint' instead.

The number is resolved with GET /phone/numbers and its phone_number filter;
an extension is matched against auto receptionists, call queues, users, common areas and
shared line groups. Auto receptionists follow their call handling
routing_action (IVR key actions and the no-input action when it is
forward_to_ivr); call queues list their members (business hours) and their
overflow or closed-hours action. Users, common areas, shared line groups,
voicemail and external targets end a branch. Per-node fetch errors are
recorded on the node; a repeated hop is marked cycle: true. A subtree cut
short (--max-depth, or a member list / extension lookup stopped by --max-pages)
is marked truncated: true with a truncated_reason, and the top-level truncated
flag is set. Every run reads live data (the response cache is bypassed). Exit 3
when the number or extension does not exist in the account.`),
		Example: strings.Trim(`
  zoom-phone-cli routing trace +14155550123
  zoom-phone-cli routing trace 1001 --agent
  zoom-phone-cli routing trace --number +14155550123 --max-depth 3 --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "live",
			"pp:happy-args":       "number=+14155550123",
			"pp:typed-exit-codes": "0,3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "routing trace")
			}
			inputs := 0
			input, mode := "", "auto"
			if len(args) > 0 {
				inputs++
				input = args[0]
			}
			if strings.TrimSpace(flagNumber) != "" {
				inputs++
				input, mode = flagNumber, "number"
			}
			if strings.TrimSpace(flagExt) != "" {
				inputs++
				input, mode = flagExt, "ext"
			}
			if len(args) > 1 || inputs > 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass exactly one of: a positional number-or-extension, --number or --ext"))
			}
			if strings.TrimSpace(input) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a phone number or extension is required"))
			}
			if flagMaxDepth < 1 {
				return usageErr(fmt.Errorf("--max-depth must be at least 1"))
			}
			if flagMaxPages < 1 {
				return usageErr(fmt.Errorf("--max-pages must be at least 1"))
			}
			if zoomDigits(input) == "" {
				return usageErr(fmt.Errorf("%q is not a phone number or extension number", input))
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Routing changes must show up immediately: bypass the response cache.
			c.NoCache = true
			// cmd.Context(), not boundCtx: the client applies --timeout per request.
			ctx := cmd.Context()

			maxPages := routingEffectiveMaxPages(flagMaxPages)
			resolved, err := routingResolve(ctx, c, input, mode, maxPages)
			if err != nil {
				return err
			}

			result := routingTraceResult{
				Input:    strings.TrimSpace(input),
				Resolved: resolved,
				MaxDepth: flagMaxDepth,
				MaxPages: maxPages,
			}
			if resolved.Type == "unassigned" {
				result.Note = "the number exists in the account but is not assigned, so inbound calls do not route anywhere"
			} else {
				tracer := &routingTracer{
					ctx:      ctx,
					c:        c,
					maxDepth: flagMaxDepth,
					maxPages: maxPages,
					cache:    map[string]routingCachedGet{},
				}
				result.Business = tracer.trace(resolved, routingHoursBusiness)
				result.Closed = tracer.trace(resolved, routingHoursClosed)
				result.Truncated = routingAnyTruncated(result.Business) || routingAnyTruncated(result.Closed)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			routingPrintTrace(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&flagNumber, "number", "", "Inbound phone number in E.164 format (e.g. +14155550123)")
	cmd.Flags().StringVar(&flagExt, "ext", "", "Extension number to trace instead of a phone number")
	cmd.Flags().IntVar(&flagMaxDepth, "max-depth", 5, "Maximum routing hops to follow before marking a branch truncated")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", routingDefaultMaxPages, "Maximum list pages to scan when resolving an extension")
	return cmd
}

// routingResolve maps the input to the entity that owns it. mode is
// "number", "ext" or "auto" (E.164 or 10+ digits try the number first).
func routingResolve(ctx context.Context, c *client.Client, input, mode string, maxPages int) (routingResolved, error) {
	trimmed := strings.TrimSpace(input)
	digits := zoomDigits(trimmed)
	// One parser (zoomPhoneE164) for the number the trace resolves; a bare
	// number with no known country is looked up as written with a +.
	e164, ok := zoomPhoneE164(trimmed)
	if !ok {
		e164 = "+" + digits
	}
	switch {
	case mode == "number":
		return routingResolveNumber(ctx, c, e164, maxPages)
	case mode == "ext":
		return routingScanEntities(ctx, c, digits, "", maxPages)
	case strings.HasPrefix(trimmed, "+"):
		return routingResolveNumber(ctx, c, e164, maxPages)
	case len(digits) >= 10:
		r, err := routingResolveNumber(ctx, c, e164, maxPages)
		if err == nil || !routingIsNotFound(err) {
			return r, err
		}
		return routingScanEntities(ctx, c, digits, "", maxPages)
	default:
		return routingScanEntities(ctx, c, digits, "", maxPages)
	}
}

func routingResolveNumber(ctx context.Context, c *client.Client, e164 string, maxPages int) (routingResolved, error) {
	raw, err := c.Get(ctx, "/phone/numbers", map[string]string{"phone_number": e164, "type": "all"})
	if err != nil {
		// /phone/numbers is deprecated in the spec; fall back to scanning the
		// entities that carry phone_numbers[] before giving up.
		if r, scanErr := routingScanEntities(ctx, c, "", e164, maxPages); scanErr == nil {
			return r, nil
		}
		return routingResolved{}, classifyAPIErrorOnly(err)
	}
	var resp struct {
		PhoneNumbers []struct {
			ID       string `json:"id"`
			Number   string `json:"number"`
			Assignee *struct {
				ExtensionNumber zoomFlexString `json:"extension_number"`
				ID              string         `json:"id"`
				Name            string         `json:"name"`
				Type            string         `json:"type"`
			} `json:"assignee"`
		} `json:"phone_numbers"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return routingResolved{}, apiErr(fmt.Errorf("decode /phone/numbers: %w", err))
	}
	want := zoomPhoneKey(e164)
	for _, pn := range resp.PhoneNumbers {
		if zoomPhoneKey(pn.Number) != want {
			continue
		}
		if pn.Assignee == nil || pn.Assignee.Type == "" {
			return routingResolved{Type: "unassigned", PhoneNumber: pn.Number}, nil
		}
		return routingResolved{
			Type:            pn.Assignee.Type,
			ID:              pn.Assignee.ID,
			Name:            pn.Assignee.Name,
			ExtensionNumber: pn.Assignee.ExtensionNumber.String(),
			PhoneNumber:     pn.Number,
		}, nil
	}
	return routingResolved{}, notFoundErr(fmt.Errorf("phone number %s was not found in this Zoom Phone account", e164))
}

type routingScanSpec struct {
	typ        string
	path       string
	itemsKey   string
	pageSize   int
	keywordExt bool // keyword matches extension numbers
	keywordNum bool // keyword matches phone numbers
}

var routingScanSpecs = []routingScanSpec{
	{routingTypeAutoReceptionist, "/phone/auto_receptionists", "auto_receptionists", 100, false, false},
	{routingTypeCallQueue, "/phone/call_queues", "call_queues", 100, true, true},
	{routingTypeUser, "/phone/users", "users", 100, true, true},
	{routingTypeCommonArea, "/phone/common_areas", "common_areas", 100, true, false},
	{routingTypeSharedLineGroup, "/phone/shared_line_groups", "shared_line_groups", 300, true, true},
}

// routingScanEntities finds the entity whose extension_number equals ext, or
// whose phone_numbers[] contains phone. Exactly one of ext/phone is set.
func routingScanEntities(ctx context.Context, c *client.Client, ext, phone string, maxPages int) (routingResolved, error) {
	phoneKey := zoomPhoneKey(phone)
	keyword := ext
	if phone != "" {
		keyword = zoomDigits(phone)
	}
	var firstErr error
	var truncated []string
	for _, spec := range routingScanSpecs {
		params := map[string]string{}
		// Keyword filters need at least 3 characters on most list endpoints.
		if len(keyword) >= 3 && ((ext != "" && spec.keywordExt) || (phone != "" && spec.keywordNum)) {
			params["keyword"] = keyword
		}
		var hit *routingResolved
		stop := func(item json.RawMessage) bool {
			var e struct {
				ID              string             `json:"id"`
				Name            string             `json:"name"`
				DisplayName     string             `json:"display_name"`
				ExtensionNumber zoomFlexString     `json:"extension_number"`
				PhoneNumbers    []routingNumberRef `json:"phone_numbers"`
			}
			if json.Unmarshal(item, &e) != nil {
				return false
			}
			matched := false
			matchedNumber := ""
			if ext != "" && e.ExtensionNumber.String() == ext {
				matched = true
			}
			if phone != "" {
				for _, pn := range e.PhoneNumbers {
					if zoomPhoneKey(pn.Number) == phoneKey {
						matched, matchedNumber = true, pn.Number
						break
					}
				}
			}
			if !matched {
				return false
			}
			name := e.Name
			if name == "" {
				name = e.DisplayName
			}
			hit = &routingResolved{Type: spec.typ, ID: e.ID, Name: name, ExtensionNumber: e.ExtensionNumber.String(), PhoneNumber: matchedNumber}
			return true
		}
		pr, err := routingListAll(ctx, c, spec.path, spec.itemsKey, params, spec.pageSize, maxPages, stop)
		if hit != nil {
			return *hit, nil
		}
		if pr.Truncated {
			truncated = append(truncated, spec.itemsKey)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return routingResolved{}, classifyAPIErrorOnly(firstErr)
	}
	if len(truncated) > 0 {
		what := "extension " + ext
		if ext == "" {
			what = "phone number " + phone
		}
		return routingResolved{}, notFoundErr(fmt.Errorf("%s was not found in the scanned pages; the %s list(s) stopped at --max-pages %d, raise --max-pages to scan further", what, strings.Join(truncated, ", "), maxPages))
	}
	if ext != "" {
		return routingResolved{}, notFoundErr(fmt.Errorf("extension %s was not found among auto receptionists, call queues, users, common areas or shared line groups", ext))
	}
	return routingResolved{}, notFoundErr(fmt.Errorf("phone number %s was not found in this Zoom Phone account", phone))
}

func (t *routingTracer) get(path string, params map[string]string) (json.RawMessage, error) {
	key := routingCacheKey(path, params)
	if v, ok := t.cache[key]; ok {
		return v.raw, v.err
	}
	raw, err := t.c.Get(t.ctx, path, params)
	t.cache[key] = routingCachedGet{raw: raw, err: err}
	return raw, err
}

func (t *routingTracer) trace(r routingResolved, hours string) *routingNode {
	root := &routingNode{
		Type:            r.Type,
		ID:              r.ID,
		Name:            r.Name,
		ExtensionNumber: r.ExtensionNumber,
		PhoneNumber:     r.PhoneNumber,
		Action:          "inbound",
		Children:        []*routingNode{},
	}
	t.nodes = 1
	t.expand(root, hours, 0, map[string]bool{})
	return root
}

func (t *routingTracer) expand(n *routingNode, hours string, depth int, path map[string]bool) {
	if n.Type != routingTypeAutoReceptionist && n.Type != routingTypeCallQueue {
		return
	}
	if n.ID == "" || n.noFollow {
		return
	}
	key := n.Type + ":" + n.ID
	if path[key] {
		n.Cycle = true
		return
	}
	if depth >= t.maxDepth {
		n.Truncated, n.TruncatedReason = true, "max_depth"
		return
	}
	if t.nodes >= routingTraceMaxNodes {
		n.Truncated, n.TruncatedReason = true, "node_budget"
		return
	}
	path[key] = true
	defer delete(path, key)
	if n.Type == routingTypeAutoReceptionist {
		t.expandAutoReceptionist(n, hours)
	} else {
		t.expandCallQueue(n, hours)
	}
	t.nodes += len(n.Children)
	for _, child := range n.Children {
		t.expand(child, hours, depth+1, path)
	}
}

func (t *routingTracer) expandAutoReceptionist(n *routingNode, hours string) {
	base := "/phone/auto_receptionists/" + url.PathEscape(n.ID)
	route := "forward_to_ivr"
	var handling routingARHandling
	raw, err := t.get(base+"/call_handling/settings", map[string]string{"hour_type": hours})
	switch {
	case err != nil:
		n.Error = routingJoinErr(n.Error, "call handling: "+routingErrText(err))
	case json.Unmarshal(raw, &handling) != nil:
		n.Error = routingJoinErr(n.Error, "call handling: undecodable response")
	case handling.CallHandlingSetting.RoutingAction != "":
		route = handling.CallHandlingSetting.RoutingAction
	}
	if route != "forward_to_ivr" {
		n.Children = append(n.Children, t.handlingChild(route, handling.CallHandlingSetting.RoutingActionSetting, n))
		return
	}
	raw, err = t.get(base+"/ivr", map[string]string{"hours_type": hours})
	if err != nil {
		n.Error = routingJoinErr(n.Error, "ivr: "+routingErrText(err))
		return
	}
	var ivr routingIVR
	if err := json.Unmarshal(raw, &ivr); err != nil {
		n.Error = routingJoinErr(n.Error, "ivr: undecodable response")
		return
	}
	for _, ka := range ivr.KeyActions {
		child := routingIVRChild(int(ka.Action), ka.Target, n, false)
		if child == nil {
			continue
		}
		child.Key = ka.Key
		t.resolveTarget(child)
		n.Children = append(n.Children, child)
	}
	if ivr.NoAction != nil {
		if child := routingIVRChild(int(ivr.NoAction.Action), ivr.NoAction.ForwardTo, n, true); child != nil {
			child.Key = "no_input"
			t.resolveTarget(child)
			n.Children = append(n.Children, child)
		}
	}
}

func routingIVRChild(code int, target routingIVRTarget, parent *routingNode, noInput bool) *routingNode {
	info := routingIVRActionInfo(code, noInput)
	if info.skip {
		return nil
	}
	child := &routingNode{
		Type:            info.nodeType,
		ID:              target.ID,
		Name:            target.DisplayName,
		ExtensionNumber: target.ExtensionNumber.String(),
		ExtensionID:     target.ExtensionID,
		PhoneNumber:     target.PhoneNumber,
		Action:          info.label,
		Children:        []*routingNode{},
	}
	if code == 100 {
		// "Leave voicemail to the current extension": the auto receptionist itself.
		child.ID, child.Name, child.ExtensionNumber = parent.ID, parent.Name, parent.ExtensionNumber
	}
	return child
}

func (t *routingTracer) expandCallQueue(n *routingNode, hours string) {
	base := "/phone/call_queues/" + url.PathEscape(n.ID)
	// Call distribution (ringing members) applies to business hours only;
	// the call handling action applies to every hour type.
	if hours == routingHoursBusiness {
		members, truncated, err := t.queueMembers(base + "/members")
		if err != nil {
			n.Error = routingJoinErr(n.Error, "members: "+routingErrText(err))
		}
		if truncated {
			n.Truncated, n.TruncatedReason = true, "member_list_max_pages"
		}
		for _, m := range members {
			typ := routingTypeUser
			if m.Level == routingTypeCommonArea {
				typ = routingTypeCommonArea
			}
			action := "ring_member"
			if m.ReceiveCall != nil && !*m.ReceiveCall {
				action = "member_not_receiving_calls"
			}
			n.Children = append(n.Children, &routingNode{
				Type:        typ,
				ID:          m.ID,
				Name:        m.Name,
				ExtensionID: m.ExtensionID,
				Action:      action,
				Children:    []*routingNode{},
			})
		}
	}
	raw, err := t.get(base+"/call_handling/settings", map[string]string{"hour_type": hours})
	if err != nil {
		n.Error = routingJoinErr(n.Error, "call handling: "+routingErrText(err))
		return
	}
	var handling routingCQHandling
	if err := json.Unmarshal(raw, &handling); err != nil {
		n.Error = routingJoinErr(n.Error, "call handling: undecodable response")
		return
	}
	action := handling.CallHandlingSetting.OverflowAction
	if action == "" {
		return
	}
	child := t.handlingChild(action, handling.CallHandlingSetting.OverflowActionSetting, n)
	if hours == routingHoursBusiness {
		child.Action = "overflow_" + action
	}
	n.Children = append(n.Children, child)
}

// queueMembers walks a call queue's member list; truncated reports that
// --max-pages stopped the walk while more members remained.
func (t *routingTracer) queueMembers(path string) ([]routingQueueMember, bool, error) {
	key := "members|" + path
	if v, ok := t.cache[key]; ok {
		var cached struct {
			Members   []routingQueueMember `json:"members"`
			Truncated bool                 `json:"truncated"`
		}
		_ = json.Unmarshal(v.raw, &cached)
		return cached.Members, cached.Truncated, v.err
	}
	res, err := routingListAll(t.ctx, t.c, path, "call_queue_members", nil, 0, t.maxPages, nil)
	members := make([]routingQueueMember, 0, len(res.Items))
	for _, it := range res.Items {
		var m routingQueueMember
		if json.Unmarshal(it, &m) == nil {
			members = append(members, m)
		}
	}
	raw, _ := json.Marshal(map[string]any{"members": members, "truncated": res.Truncated})
	t.cache[key] = routingCachedGet{raw: raw, err: err}
	return members, res.Truncated, err
}

// handlingChild converts a call handling routing/overflow action into a node.
func (t *routingTracer) handlingChild(action string, s routingHandlingSetting, parent *routingNode) *routingNode {
	child := &routingNode{Action: action, Children: []*routingNode{}}
	switch action {
	case "forward_to_voicemail", "forward_to_voicemail_access":
		child.Type = "voicemail"
		if ref := s.LeaveVoicemailTo; ref != nil && (ref.ExtensionID != "" || ref.ExtensionName != "") {
			child.Name, child.ExtensionNumber, child.ExtensionID = ref.ExtensionName, ref.ExtensionNumber.String(), ref.ExtensionID
		} else {
			child.ID, child.Name, child.ExtensionNumber = parent.ID, parent.Name, parent.ExtensionNumber
		}
	case "forward_to_extension":
		ref := s.ForwardToExtension
		if ref == nil {
			child.Type = "unknown"
			child.Error = "forward_to_extension has no target"
			break
		}
		child.Type = ref.ExtensionType
		child.Name, child.ExtensionNumber, child.ExtensionID = ref.ExtensionName, ref.ExtensionNumber.String(), ref.ExtensionID
		t.resolveTarget(child)
	case "forward_to_external_number":
		child.Type, child.PhoneNumber = "phoneNumber", s.ForwardPhoneNumber
	case "forward_to_external_contact":
		child.Type = "externalContact"
		if ec := s.ForwardToExternalContact; ec != nil {
			child.ID, child.Name = ec.ExternalContactID, ec.ExternalContactName
			if len(ec.PhoneNumbers) > 0 {
				child.PhoneNumber = ec.PhoneNumbers[0]
			}
		}
	case "forward_to_partner_contact_center":
		child.Type = "partnerContactCenter"
		if p := s.ForwardToPartnerContactCenter; p != nil {
			child.ID, child.Name = p.PartnerContactCenterID, p.PartnerContactCenterName
		}
	case "forward_to_zoom_contact_center":
		child.Type = "zoomContactCenter"
		child.PhoneNumber, child.Name = s.ZoomContactCenterNumber, s.ZoomContactCenterFlowName
		if z := s.ForwardToZoomContactCenter; z != nil {
			child.PhoneNumber, child.Name = z.ZoomContactCenterNumber, z.ZoomContactCenterFlowName
		}
	case "forward_to_teams":
		child.Type = "teams"
		if tm := s.ForwardToTeams; tm != nil {
			child.ID, child.Name = tm.TeamsAppID, tm.TeamsVoiceAppName
		}
	case "play_message_then_disconnect":
		child.Type = "message"
	case "disconnect":
		child.Type = "disconnect"
	default:
		child.Type = "unknown"
	}
	return child
}

// resolveTarget applies the stable-identifier rule to an auto receptionist or
// call queue target (IVR key, no-input action or call-handling forward): the
// id and extension_id are authoritative, the extension number is used only
// when neither is present, and a conflict or a target missing from a complete
// list is recorded on the node and not followed. When the list could not be
// read completely, a stated id is still followed (it cannot be disproved).
func (t *routingTracer) resolveTarget(n *routingNode) {
	if n.Type != routingTypeAutoReceptionist && n.Type != routingTypeCallQueue {
		return
	}
	if n.ID == "" && n.ExtensionID == "" && n.ExtensionNumber == "" {
		n.Error = routingJoinErr(n.Error, "target has no id, extension_id or extension number")
		n.noFollow = true
		return
	}
	idx := t.loadIndex(n.Type)
	e, found, conflict := idx.ix.lookup(routingIVRTarget{ID: n.ID, ExtensionID: n.ExtensionID, ExtensionNumber: zoomFlexString(n.ExtensionNumber)})
	switch {
	case conflict != "":
		n.Error = routingJoinErr(n.Error, "conflicting target identifiers: "+conflict)
		n.noFollow = true
	case found:
		n.ID = e.id
		if n.Name == "" {
			n.Name = e.name
		}
	case idx.complete:
		n.Error = routingJoinErr(n.Error, fmt.Sprintf("target %s is not among the account's %s (deleted?)", routingFirstNonEmpty(n.ID, n.ExtensionID, n.ExtensionNumber), n.Type+"s"))
		n.noFollow = true
	case n.ID != "":
		// The list is incomplete, so the stated id cannot be disproved: follow it.
	case idx.truncated:
		// The target may sit on a list page --max-pages did not reach.
		n.Truncated, n.TruncatedReason = true, "extension_index_max_pages"
		n.Error = routingJoinErr(n.Error, "could not resolve the target within the scanned pages; raise --max-pages")
		n.noFollow = true
	default:
		msg := "could not resolve the target to an id"
		if idx.err != nil {
			msg += ": " + routingErrText(idx.err)
		}
		n.Error = routingJoinErr(n.Error, msg)
		n.noFollow = true
	}
}

// loadIndex walks the auto receptionist or call queue list once per run.
func (t *routingTracer) loadIndex(typ string) *routingTraceIndex {
	if t.indexes == nil {
		t.indexes = map[string]*routingTraceIndex{}
	}
	if idx, ok := t.indexes[typ]; ok {
		return idx
	}
	idx := &routingTraceIndex{ix: routingEntityIndex{}}
	t.indexes[typ] = idx
	for _, spec := range routingScanSpecs {
		if spec.typ != typ {
			continue
		}
		res, err := routingListAll(t.ctx, t.c, spec.path, spec.itemsKey, nil, spec.pageSize, t.maxPages, nil)
		idx.err, idx.truncated = err, res.Truncated
		idx.complete = err == nil && !res.Truncated
		for _, it := range res.Items {
			var e struct {
				ID              string         `json:"id"`
				Name            string         `json:"name"`
				ExtensionID     string         `json:"extension_id"`
				ExtensionNumber zoomFlexString `json:"extension_number"`
			}
			if json.Unmarshal(it, &e) == nil && e.ID != "" {
				idx.ix.add(routingEntityInfo{typ: typ, id: e.ID, name: e.Name, ext: e.ExtensionNumber.String()}, e.ExtensionID)
			}
		}
	}
	return idx
}

func routingPrintTrace(w io.Writer, r routingTraceResult) {
	fmt.Fprintf(w, "Input: %s -> %s\n", r.Input, routingNodeLabel(&routingNode{
		Type: r.Resolved.Type, Name: r.Resolved.Name, ExtensionNumber: r.Resolved.ExtensionNumber, ID: r.Resolved.ID,
	}))
	if r.Note != "" {
		fmt.Fprintf(w, "Note: %s\n", r.Note)
	}
	for _, section := range []struct {
		title string
		node  *routingNode
	}{{"Business hours", r.Business}, {"Closed hours", r.Closed}} {
		if section.node == nil {
			continue
		}
		fmt.Fprintf(w, "\n%s:\n", section.title)
		routingPrintNode(w, section.node, 1)
	}
}

func routingPrintNode(w io.Writer, n *routingNode, depth int) {
	prefix := strings.Repeat("  ", depth)
	line := ""
	if n.Key != "" {
		line += "[" + n.Key + "] "
	}
	line += n.Action + " -> " + routingNodeLabel(n)
	if n.Cycle {
		line += "  (cycle)"
	}
	if n.Truncated {
		line += "  (truncated: " + routingFirstNonEmpty(n.TruncatedReason, "incomplete") + ")"
	}
	if n.Error != "" {
		line += "  [error: " + n.Error + "]"
	}
	fmt.Fprintln(w, prefix+line)
	for _, ch := range n.Children {
		routingPrintNode(w, ch, depth+1)
	}
}

func routingNodeLabel(n *routingNode) string {
	label := n.Type
	if n.Name != "" {
		label += " " + n.Name
	}
	if n.ExtensionNumber != "" {
		label += " (ext " + n.ExtensionNumber + ")"
	}
	if n.PhoneNumber != "" && n.Type != routingTypeAutoReceptionist && n.Type != routingTypeCallQueue {
		label += " " + n.PhoneNumber
	}
	return label
}

func routingAnyTruncated(n *routingNode) bool {
	if n == nil {
		return false
	}
	if n.Truncated {
		return true
	}
	for _, ch := range n.Children {
		if routingAnyTruncated(ch) {
			return true
		}
	}
	return false
}
