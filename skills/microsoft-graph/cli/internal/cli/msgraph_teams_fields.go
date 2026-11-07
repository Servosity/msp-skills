// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// microsoft-graph handfix (teams-phone-agent-fields): --agent and --compact
// keep only high-gravity names, so a Teams Phone read dropped the number, its
// type and the voice settings it exists to show. These fields stay in compact
// output for phone-numbers and teams-users (numberAssignment and
// teamsUserConfiguration, Graph v1.0).
var (
	phoneNumberAgentFields = []string{"telephoneNumber", "numberType", "assignmentCategory", "activationState", "assignmentStatus", "assignmentTargetId"}
	teamsUserAgentFields   = []string{"userPrincipalName", "isEnterpriseVoiceEnabled", "accountType", "telephoneNumbers", "effectivePolicyAssignments"}
)
