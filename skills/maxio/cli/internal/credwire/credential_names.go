// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

// credentialFlags is every command-line flag that carries a card or bank secret
// or a single-use payment token: the flags cliutil.SecretFlagName reads as a
// credential, the vault reference flags aside. A vault reference names a card
// stored at a gateway and is not a secret the operator holds; a gateway's value
// can be as short as "1", and held it would withhold any output with that word.
// TestCredentialFlagsMatchTheNameRule keeps the list complete.
var credentialFlags = []string{
	"--authorization",
	"--payment-profile-bank-account-number",
	"--payment-profile-bank-iban",
	"--payment-profile-chargify-token",
	"--payment-profile-cvv",
	"--payment-profile-full-number",
	"--payment-profile-payment-method-nonce",
	"--subscription-bank-account-attributes-bank-account-number",
	"--subscription-bank-account-attributes-bank-iban",
	"--subscription-bank-account-attributes-chargify-token",
	"--subscription-credit-card-attributes-chargify-token",
	"--subscription-credit-card-attributes-cvv",
	"--subscription-credit-card-attributes-full-number",
	"--subscription-credit-card-attributes-payment-method-nonce",
	"--subscription-group-credit-card-attributes-chargify-token",
	"--subscription-group-credit-card-attributes-cvv",
	"--subscription-group-credit-card-attributes-full-number",
	"--subscription-payment-profile-attributes-chargify-token",
	"--subscription-payment-profile-attributes-cvv",
	"--subscription-payment-profile-attributes-full-number",
	"--subscription-payment-profile-attributes-payment-method-nonce",
}

// credentialFields are the Maxio request and response names that carry a secret;
// a request also holds "token", which no Maxio field uses: a --deliver webhook
// URL whose query carries token=... is a credential the guard refuses to send
// (TestCredentialFieldsMatchTheNameRule). The vault references and webhook
// signatures the name rule also reads are redacted from every response and not
// held, for the reason above.
var credentialFields = []string{
	"bank_account_number",
	"bank_iban",
	"chargify_token",
	"cvv",
	"full_number",
	"payment_method_nonce",
	"self_service_page_token",
}

// credentialArguments are the MCP arguments held before dispatch: those fields
// wherever the execute tool's body or params carry them, and the key's own names.
var credentialArguments = []string{
	"bank_account_number",
	"bank_iban",
	"chargify_token",
	"cvv",
	"full_number",
	"payment_method_nonce",
	"self_service_page_token",
	"Authorization",
	"api_key",
	"apikey",
}
