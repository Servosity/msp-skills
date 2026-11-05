// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"slices"
	"strings"
	"testing"
)

// Class credential-field-not-recognized: a credential name matches in every
// spelling (camelCase, snake_case, kebab-case flags and headers, any case),
// and the public half of a credential pair (a clientId, an access key ID)
// counts as a credential; a resource identifier does not.
func TestCredentialNamesMatchInEverySpelling(t *testing.T) {
	for _, name := range []string{
		"clientSecret", "client_secret", "client-secret", "password", "psk", "key", "Key", "secretAccessKey",
		"clientId", "client_id", "client-id", "ClientID", "CLIENT_ID",
		"apiKey", "api_key", "api-key", "x-api-key", "X-API-KEY",
		"accessKeyId", "access_key_id", "access-key-id",
		"private-key", "license-key", "pre-shared-key", "pass-phrase", "passphrase",
	} {
		if !SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"id", "name", "keyId", "creatorKeyId", "client", "clientProfile", "client-device-type",
		"password_complexity", "passwordComplexity", "keyName", "deviceKey", "psk-id-prefix", "originId",
	} {
		if SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = true, want false", name)
		}
	}
}

// A response member under a credential name is held and replaced by name, at any
// depth and in every duplicate of its key, and the record stays readable; ordinary
// Exchange identifiers are neither.
func TestRedactBodyAndHoldByName(t *testing.T) {
	body := []byte(`{"value":[{"Identity":"rosa@example.com","Guid":"11111111-2222-3333-4444-555555555555","Password":"ReplacedUnit-1","Password":"ReplacedUnit-2","nested":{"client_secret":"ReplacedUnit-3"}}]}`)
	got := recordHolds(func() { HoldCredentialFields(body) })
	for _, want := range []string{"ReplacedUnit-1", "ReplacedUnit-2", "ReplacedUnit-3"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s not held: %q", want, got)
		}
	}
	for _, not := range []string{"rosa@example.com", "11111111-2222-3333-4444-555555555555"} {
		if slices.Contains(got, not) {
			t.Errorf("%s held, but it is an ordinary identifier", not)
		}
	}
	out := string(RedactBody(body))
	if strings.Contains(out, "ReplacedUnit") || !strings.Contains(out, "rosa@example.com") || !strings.Contains(out, RedactedValue) {
		t.Errorf("RedactBody: %s", out)
	}
}

// No property of a realistic Get-Mailbox or Get-InboxRule row is a credential name,
// or every listing would be withheld.
func TestExchangeProperties(t *testing.T) {
	for _, name := range []string{
		"Identity", "Guid", "ExchangeGuid", "ExternalDirectoryObjectId", "Name", "Alias", "DisplayName", "UserPrincipalName",
		"PrimarySmtpAddress", "EmailAddresses", "RecipientTypeDetails", "ForwardingSmtpAddress", "ForwardingAddress",
		"DeliverToMailboxAndForward", "HiddenFromAddressListsEnabled", "LitigationHoldEnabled", "WhenChanged", "WhenCreated",
		"AccountDisabled", "IsMailboxEnabled", "RulesQuota", "ArchiveGuid", "ProhibitSendQuota", "UsageLocation",
		"RuleIdentity", "Enabled", "Priority", "From", "SubjectContainsWords", "ForwardTo", "RedirectTo", "DeleteMessage",
		"MoveToFolder", "StopProcessingRules", "MailboxOwnerId", "Description", "AccessRights", "Trustee", "User",
		"DomainName", "DomainType", "SendAs", "EwsEnabled", "OAuthEnabled", "SmtpClientAuthenticationDisabled",
		"BypassModerationFromSendersOrMembers", "client_profile", "credential_ref_name_only_not_exchange",
	} {
		if SecretFieldName(name) && name != "credential_ref_name_only_not_exchange" {
			t.Errorf("SecretFieldName(%q) = true: this ordinary Exchange property would be held", name)
		}
	}
}
