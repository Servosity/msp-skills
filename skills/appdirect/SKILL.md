---
name: appdirect
description: "Every documented AppDirect marketplace operation in one binary, plus offline sync and billing-reconciliation joins. Trigger phrases: `reconcile appdirect billing`, `which appdirect payments failed this week`, `what changed in appdirect subscriptions`, `show my appdirect pipeline`, `appdirect company 360`, `use appdirect`, `run appdirect-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "AppDirect"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - appdirect-cli
    install:
      - kind: go
        bins: [appdirect-cli]
        module: github.com/mvanhorn/printing-press-library/library/commerce/appdirect/cmd/appdirect-cli
---

# AppDirect  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `appdirect-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install appdirect --cli-only
   ```
2. Verify: `appdirect-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/commerce/appdirect/cmd/appdirect-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

AppDirect partners manage thousands of subscriptions across hundreds of companies through a click-heavy console or hand-rolled curl with hourly-expiring OAuth tokens. This CLI mints and caches tokens invisibly, wraps the full marketplace REST surface (subscriptions, companies, users, billing, assisted sales, catalog), and syncs it all to local SQLite so commands like 'reconcile', 'subs changed', and 'pipeline' answer cross-entity questions no console screen can.

## When to Use This CLI

Use this CLI for AppDirect marketplace operations: listing and managing subscriptions, companies, users, and memberships; pulling invoices and payments; reconciling billing; and working assisted-sales pipelines. It shines on cross-entity questions (billing mismatches, churn, stale deals) because everything syncs to a local SQLite store that supports search, SQL, and joins.

## Anti-triggers

Do not use this CLI for:
- Building or theming a marketplace storefront - that is @appdirect/sfb-toolkit, not this CLI
- Developing ISV product connectors / integration event handlers - use the AppDirect connector SDKs
- End-user purchases on a marketplace you do not operate - this CLI needs partner API credentials
- AppDirect GraphQL-only surfaces (newer product/catalog graph queries) - this CLI wraps the REST API

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local joins that close the books
- **`reconcile`**  -  Flag active subscriptions with no matching invoice, orphan invoices, and failed or unpaid payments across every company before month-close.

  _Reach for this when asked whether billing matches reality across the marketplace  -  it answers in one call what otherwise takes hundreds of per-company console lookups._

  ```bash
  appdirect-cli reconcile --since 30d --agent
  ```
- **`payments unpaid`**  -  List failed or unavailable-gateway payments across all companies, sorted by amount and age.

  _Use this for the weekly failed-payment chase instead of paging through company billing screens._

  ```bash
  appdirect-cli payments unpaid --since 7d --json
  ```
- **`company show`**  -  One customer's full picture - users, subscriptions, invoices, and open opportunities - in a single view.

  _Pick this for onboarding or support questions about a specific customer instead of four separate API calls._

  ```bash
  appdirect-cli company show 12345678-aaaa-bbbb-cccc-1234567890ab --json
  ```

### Change radar
- **`subs changed`**  -  See subscriptions created, ended, or in an inactive status (suspended/cancelled/failed) across all companies in a time window.

  _Use this for churn checks and weekly change review across the whole marketplace._

  ```bash
  appdirect-cli subs changed --since 7d --json
  ```

### Pipeline as data
- **`pipeline`**  -  Roll up the assisted-sales pipeline by status or owner with opportunity counts and ages.

  _Use this when asked for a pipeline rollup by status or owner instead of opening each opportunity._

  ```bash
  appdirect-cli pipeline --group-by status --agent
  ```
- **`pipeline stale`**  -  Find open opportunities created more than N days ago, oldest first.

  _Use this to surface stalled deals before they die quietly._

  ```bash
  appdirect-cli pipeline stale --days 14 --json
  ```

## Command Reference

**account**  -  Manage account

- `appdirect-cli account resource-company-create-active-user-post`  -  Create a marketplace user with membership in the given company.
- `appdirect-cli account resource-company-create-company-membership-patch`  -  Update role assignments for one or more company memberships (users).
- `appdirect-cli account resource-company-create-company-membership-post`  -  Add a new or existing user as a member of a marketplace company.
- `appdirect-cli account resource-company-create-company-post`  -  Create a new marketplace company
- `appdirect-cli account resource-company-delete-company-membership-delete`  -  Delete a marketplace user's company membership.
- `appdirect-cli account resource-company-get-invited-users-get`  -  Retrieve a marketplace company's user invitations.
- `appdirect-cli account resource-company-get-verified-domains-get`  -  Retrieve a marketplace company's verified domains.
- `appdirect-cli account resource-company-invite-csvusers-post`  -  Invite multiple users to a marketplace company using a CSV file.
- `appdirect-cli account resource-company-invite-managed-user-post`  -  Invite a managed user as a member of a marketplace company.
- `appdirect-cli account resource-company-invite-users-post`  -  Invite multiple users to a marketplace company using list of Invitation resources.
- `appdirect-cli account resource-company-partial-update-company-membership-patch`  -  Enable or disable marketplace user's company membership.
- `appdirect-cli account resource-company-patch-company-patch`  -  Partially update marketplace company account information.
- `appdirect-cli account resource-company-read-all-companies-get`  -  Retrieve all marketplace companies. Rate limit: Bucket size is 20 requests, with a leak rate of 5 requests per second.
- `appdirect-cli account resource-company-read-assignable-roles-get`  -  Retrieve marketplace roles that can be assigned by this company membership.
- `appdirect-cli account resource-company-read-company-activities-get`  -  Retrieve company or user activities.
- `appdirect-cli account resource-company-read-company-get`  -  Retrieve a marketplace company by ID or external ID.
- `appdirect-cli account resource-company-read-company-membership-get`  -  Retrieve a marketplace user's company membership information.
- `appdirect-cli account resource-company-read-company-memberships-get`  -  List a marketplace company's user memberships.
- `appdirect-cli account resource-company-read-domain-get`  -  Get a specific company domain
- `appdirect-cli account resource-company-read-domains-get`  -  Retrieve a marketplace company's domains.
- `appdirect-cli account resource-company-read-user-assignments-get`  -  Retrieve a marketplace user's application assignments. Returns assignments that are not FAILED or CANCELLED.
- `appdirect-cli account resource-company-read-user-checklist-get`  -  Retrieve a marketplace user's checklist.
- `appdirect-cli account resource-company-request-purchase-post`  -  Request to purchase an application for the given marketplace company and user.
- `appdirect-cli account resource-company-revoke-invitation-delete`  -  Revoke a user's invitiation to a marketplace company.
- `appdirect-cli account resource-company-update-company-membership-put`  -  Update a marketplace user's company membership.
- `appdirect-cli account resource-company-update-company-picture-put`  -  Update a marketplace company’s profile picture, for example, with an image of a logo.
- `appdirect-cli account resource-company-update-company-put`  -  Update a marketplace company.
- `appdirect-cli account resource-group-batch-create-or-update-group-memberships-post`  -  Create batched group memberships change requests.
- `appdirect-cli account resource-group-create-post`  -  Create a user group for a marketplace company.
- `appdirect-cli account resource-group-delete-delete`  -  Delete a user group for the requested marketplace company.
- `appdirect-cli account resource-group-delete-group-membership-delete`  -  Delete a user from a marketplace company's user group.
- `appdirect-cli account resource-group-read-all-get`  -  Retrieve a marketplace company's user groups.
- `appdirect-cli account resource-group-read-company-groups-for-user-get`  -  Retrieve the list of groups the company membership is in.
- `appdirect-cli account resource-group-read-get`  -  Retrieve a marketplace company's user group.
- `appdirect-cli account resource-group-read-group-membership-get`  -  Retrieve a user/company membership resource.
- `appdirect-cli account resource-group-read-group-memberships-get`  -  List members of a user group for the requested marketplace company.
- `appdirect-cli account resource-group-save-group-membership-put`  -  Add a user to marketplace company's user group.
- `appdirect-cli account resource-group-update-put`  -  Update user group for the requested marketplace company.
- `appdirect-cli account resource-invitation-update-invitation-with-registration-post`  -  Accept a user's invitation to a marketplace company
- `appdirect-cli account resource-my-apps-read-app-by-pending-id`  -  Retrieve a marketplace user's application when assignment is pending.
- `appdirect-cli account resource-my-apps-read-app-by-user-entitlement`  -  Retrieve a marketplace user's application when assignment is complete.
- `appdirect-cli account resource-my-apps-read-apps`  -  List all applications assigned to a marketplace company user.
- `appdirect-cli account resource-my-apps-update-apps-order`  -  Update the order in which applications will should show up on the MyApps page.
- `appdirect-cli account resource-read-company-assignable-roles-get`  -  Retrieves a list of marketplace roles that the current
- `appdirect-cli account resource-subscription-create-subscription-assignment-post`  -  Create an application assignment for a marketplace user.
- `appdirect-cli account resource-subscription-delete-subscription-assignment-delete`  -  Delete a marketplace user's application assignment.
- `appdirect-cli account resource-subscription-read-saml-certificate-get`  -  Retrieve a subscriptions's public SAML verification certificate.
- `appdirect-cli account resource-subscription-read-saml-info-get`  -  Retrieve a subscription's SAML metadata.
- `appdirect-cli account resource-subscription-read-subscription-assignment-count-head`  -  Retrieve the number of users assigned to a subscription.
- `appdirect-cli account resource-subscription-read-subscription-assignment-get`  -  Retrieve a marketplace user's application assignment for a given subscription.
- `appdirect-cli account resource-subscription-read-subscription-assignments-get`  -  Retrieve the list of marketplace application assignments for the given subscription.
- `appdirect-cli account resource-subscription-request-subscription-reactivation-post`  -  Send a subscription reactivation request. This endpoint sends a notification to the subscription owner.
- `appdirect-cli account resource-user-patch-user-profile-patch`  -  Update a marketplace user's profile. Also marks user's checklist with profile as completed.
- `appdirect-cli account resource-user-read-all-users-get`  -  Retrieves all marketplace users
- `appdirect-cli account resource-user-read-reseller-user-get`  -  Reads a reseller user's company associations
- `appdirect-cli account resource-user-read-user-get`  -  Retrieves a marketplace user by ID or external ID. If you use the ID, you can omit a prefix or use 'id:' as the prefix.
- `appdirect-cli account resource-user-read-user-memberships-get`  -  Retrieve a user's company memberships.
- `appdirect-cli account resource-user-read-user-profile-get`  -  Retrieve profile information for a marketplace user.
- `appdirect-cli account resource-user-set-temporary-password-put`  -  Set a temporary password for the given marketplace user.
- `appdirect-cli account resource-user-update-current-company-put`  -  Update a marketplace user's last used company.
- `appdirect-cli account resource-user-update-inactive-user-patch`  -  Activate a marketplace user using its associated activation token.
- `appdirect-cli account resource-user-update-user-patch`  -  Update ('patch') one or more fields in the user details.
- `appdirect-cli account resource-user-update-user-picture-put`  -  Update a marketplace user’s profile picture.
- `appdirect-cli account resource-user-update-user-profile-put`  -  Update a marketplace user's profile. Also marks user's checklist with profile as completed.
- `appdirect-cli account resource-user-update-user-put`  -  Updates a marketplace user

**app-market**  -  Manage app market

- `appdirect-cli app-market create-payment-method`  -  When using a token, the validated payment method is assigned to the current user on the current marketplace.
- `appdirect-cli app-market create-payment-method-token`  -  Calls the payment gateway to validate the payment method, which is not yet associated with a user.
- `appdirect-cli app-market create-transfer-method`  -  Creates a new Transfer Method for the requesting (authenticated) user
- `appdirect-cli app-market create-transfer-platform-configuration`  -  Create a transfer platform configuration in the marketplace of the current user
- `appdirect-cli app-market delete-payment-method`  -  The payment method is deleted from the system and cannot be used anymore.
- `appdirect-cli app-market delete-transfer-method`  -  Deletes the transfer method with the specified ID
- `appdirect-cli app-market delete-transfer-platform-configuration`  -  Deletes the specified transfer platform configuration. It cannot be used anymore
- `appdirect-cli app-market get-default-payment-methods`  -  Returns the default payment methods for the specified user in the specified company
- `appdirect-cli app-market get-payment-method-types`  -  Returns all supported Payment Method Types. These are the only types of payment method that can be created.
- `appdirect-cli app-market get-payment-methods`  -  Returns all available payment methods for the specified user in the specified company.
- `appdirect-cli app-market get-transfer-method`  -  Retrieves the Transfer Method for the given ID
- `appdirect-cli app-market get-transfer-methods`  -  Retrieves the transfer methods associated with the requesting (authenticated) user
- `appdirect-cli app-market get-transfer-platform-configuration`  -  Retrieves the transfer platform configuration by ID
- `appdirect-cli app-market get-transfer-platform-configurations`  -  Retrieves all transfer platform configurations for the marketplace of the current user
- `appdirect-cli app-market set-default-payment-method`  -  Sets a default payment method for the specified user
- `appdirect-cli app-market update-transfer-platform-configuration`  -  Updates the specified transfer platform configuration

**app-reseller**  -  Manage app reseller

- `appdirect-cli app-reseller resource-account-v1-company-api-detail-get`  -  Retrieves the current reseller company details.
- `appdirect-cli app-reseller resource-account-v1-company-api-get-get`  -  Retrieves companies depending on context: in the Reseller context
- `appdirect-cli app-reseller resource-account-v1-company-api-post-post`  -  Create a new company linked to the current reseller.
- `appdirect-cli app-reseller resource-account-v1-customer-company-association-api-delete-delete`  -  Deletes a company association.
- `appdirect-cli app-reseller resource-account-v1-customer-company-association-api-get-get`  -  Retrieves all customer company associations.
- `appdirect-cli app-reseller resource-account-v1-customer-company-association-api-post-post`  -  Creates an association (link) between a customer company and a reseller company.
- `appdirect-cli app-reseller resource-account-v1-user-api-get-get`  -  In the Reseller Manager context, this request retrieves all Resellers and Referral users.
- `appdirect-cli app-reseller resource-account-v1-user-api-post-post`  -  Create a new user in a company linked to the Reseller company.
- `appdirect-cli app-reseller resource-account-v1-user-detail-api-get-get`  -  Get a single user details
- `appdirect-cli app-reseller resource-billing-v1-assignment-api-delete`  -  Unassign a product from a user.
- `appdirect-cli app-reseller resource-billing-v1-order-uiapi-get`  -  Retrieves a list of all user orders of a linked company. You can filter the list using optional query parameters.
- `appdirect-cli app-reseller resource-billing-v1-subscription-api-delete`  -  Note: This endpoint is being deprecated.
- `appdirect-cli app-reseller resource-billing-v1-subscription-api-get`  -  Note: This endpoint is being deprecated. // Retrieves a list of all user subscriptions of a linked company.
- `appdirect-cli app-reseller resource-billing-v1-subscription-api-get-one`  -  Note: This endpoint is being deprecated. // Read a subscription by ID
- `appdirect-cli app-reseller resource-billing-v1-subscription-api-post`  -  Note: This endpoint is being deprecated.
- `appdirect-cli app-reseller resource-billing-v1-subscription-api-put`  -  Note: This endpoint is being deprecated.
- `appdirect-cli app-reseller resource-v1-payment-method-api-get`  -  Deletes a user's payment methods
- `appdirect-cli app-reseller resource-v1-payment-method-api-post`  -  Creates a payment method on behalf of the specified user
- `appdirect-cli app-reseller resource-v1-payment-methods-api-get`  -  Retrieves a list of the specified user's payment methods
- `appdirect-cli app-reseller resource-v1-subscription-assignment-api-get`  -  Retrieves a list of assignments for a given subscription. You can filter the list using optional query parameters.
- `appdirect-cli app-reseller resource-v1-subscription-assignment-api-post`  -  Assign a seat of a purchased product to a user who is a member of a linked marketplace company
- `appdirect-cli app-reseller sellable-product-editions-catalog-v1-api-get-get`  -  Get product editions, costs and markups.
- `appdirect-cli app-reseller sellable-products-catalog-v1-api-get-get`  -  Retrieve all products that a Reseller can sell.

**appdirect-sync**  -  Manage appdirect sync

- `appdirect-cli appdirect-sync resource-developer-account-create-developer-account-post`  -  This call creates a developer account. Maximum of global requests of 20 per 2 seconds.
- `appdirect-cli appdirect-sync resource-developer-account-expire-developer-account-post`  -  This call expires a developer account. Maximum of global requests of 20 per 2 seconds.
- `appdirect-cli appdirect-sync resource-user-assign-user-post`  -  This call assigns a user. Maximum of global requests of 20 per 2 seconds.
- `appdirect-cli appdirect-sync resource-user-unassign-user-post`  -  This call unassigns a user. Maximum of global requests of 20 per 2 seconds.
- `appdirect-cli appdirect-sync user-assignment-get`  -  This call retrieves a user assignment. Maximum of global requests of 20 per 2 seconds.

**appwise**  -  Manage appwise

- `appdirect-cli appwise disconnecting`  -  Disconnects an existing user account from Search.
- `appdirect-cli appwise events`  -  Publishes content events that contain new or changed data in referenced resources.
- `appdirect-cli appwise provisioning`  -  Creates a new user connection for Search.
- `appdirect-cli appwise users-index-search`  -  Search a unified index of content derived from a user's connected accounts. Requires a user-based token.

**assisted-sales**  -  Manage assisted sales

- `appdirect-cli assisted-sales add-items`  -  Add one or more items to an opportunity Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales apply-discount`  -  Apply a discount to an opportunity. More than one discount (one per call) can be applied to an opportunity.
- `appdirect-cli assisted-sales change-owner`  -  Changes the owner of an opportunity. When the owner changes, all products are removed from the opportunity.
- `appdirect-cli assisted-sales clone-opportunity`  -  Clone an existing opportunity Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales create-opportunity`  -  Creates a new opportunity Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales create-or-update-shipping-address`  -  Create or replace a shipping address for physical goods that require one on an opportunity.
- `appdirect-cli assisted-sales create-quote`  -  Creates a quote version from a quote source Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales delete-opportunities`  -  Removes a list of opportunities Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales delete-opportunity-item`  -  Removes an item from the opportunity Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales edit-item`  -  Edit an item on the opportunity. Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales execute-action`  -  Execute the action passed by the parameter on the quote version Required: Accept-Language header with Locale format.
- `appdirect-cli assisted-sales finalize-opportunity`  -  Finalize an opportunity Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales get-opportunity-validation-results`  -  Retrieve the latest validation results for an opportunity Required: Accept-Language header with Locale format.
- `appdirect-cli assisted-sales get-quote-by-id`  -  Get a quote version from a quote ID Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales get-required-fields-definitions`  -  For a customer/sales agent combination, returns the vendor-required fields for one or more products
- `appdirect-cli assisted-sales get-shipping-address`  -  Retrieve the shipping address for the opportunity Required: Accept-Language header with Locale format.
- `appdirect-cli assisted-sales read-items`  -  Lists all items in an opportunity. Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales read-opportunities`  -  Retrieve a list of opportunities.
- `appdirect-cli assisted-sales read-opportunity`  -  Read an opportunity Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales read-opportunity-summary`  -  Read the pricing summary of an opportunity Required: Accept-Language header with Locale format. For example: en-US
- `appdirect-cli assisted-sales read-pricing-plan-costs`  -  Read initial costs for a pricing plan of an opportunity item. 'Required: Accept-Language' header with Locale format.
- `appdirect-cli assisted-sales read-pricing-plan-costs-without-opportunity`  -  Read initial costs for a pricing plan of an opportunity item without specifying an opportunity.
- `appdirect-cli assisted-sales read-quotes`  -  List quote versions based on a source type and ID Required: Accept-Language header with Locale format.
- `appdirect-cli assisted-sales remove-discount`  -  Remove a discount from an opportunity. The response is an updated opportunity summary.
- `appdirect-cli assisted-sales request-opportunity-review`  -  Submit the opportunity to the manager for review. After it is submitted, the sales agent cannot modify the opportunity.
- `appdirect-cli assisted-sales update-opportunity`  -  Updates an existing opportunity: name, purchase effective date and purchase custom attributes Required
- `appdirect-cli assisted-sales update-required-fields-on-items`  -  Update required fields for all products from the same vendor on an opportunity, with identical required field values.

**billing**  -  Manage billing

- `appdirect-cli billing resource-other-change-subscription-put`  -  Change the specified subscription for the specified user and company with the provided data.
- `appdirect-cli billing resource-other-create-payment-instrument-post`  -  Create a payment instrument for the given user and company using the provided data
- `appdirect-cli billing resource-other-create-subscription-post`  -  Create a subscription for the given user and company using the provided data
- `appdirect-cli billing resource-other-delete-payment-instrument-delete`  -  Delete a payment instrument
- `appdirect-cli billing resource-other-patch-invoice-patch`  -  Currently only used to void an invoice by updating the invoice status
- `appdirect-cli billing resource-other-pay-invoice-post`  -  If the invoice is unpaid, will process the payment and return the result. Will return an exception otherwise
- `appdirect-cli billing resource-other-preview-change-subscription-put`  -  Preview the changes for the given subscription using the provided data
- `appdirect-cli billing resource-other-preview-create-subscription-post`  -  Preview creating a subscription for the given user and company using the provided data
- `appdirect-cli billing resource-other-read-company-invoices-get`  -  List all of the invoices for the given company
- `appdirect-cli billing resource-other-read-company-payments-get`  -  List all of the payments for the given company
- `appdirect-cli billing resource-other-read-company-purchase-orders-get`  -  List all of the purchase orders for the given company
- `appdirect-cli billing resource-other-read-company-subscriptions-get`  -  List all of the subscriptions for the given company
- `appdirect-cli billing resource-other-read-default-payment-instrument-get`  -  Retrieve the default payment instrument for a given user and company
- `appdirect-cli billing resource-other-read-invoice-get`  -  Retrieve an Invoice
- `appdirect-cli billing resource-other-read-invoice-payments-get`  -  List all payment data for an invoice
- `appdirect-cli billing resource-other-read-invoices-get`  -  List all invoice data
- `appdirect-cli billing resource-other-read-order-invoices-get`  -  List all purchase order invoices
- `appdirect-cli billing resource-other-read-order-payments-get`  -  List all purchase order payments
- `appdirect-cli billing resource-other-read-payment-get`  -  Retrieve a payment given its payment number
- `appdirect-cli billing resource-other-read-payment-instrument-get`  -  Retrieve a payment instrument
- `appdirect-cli billing resource-other-read-payment-invoices-get`  -  List all invoices attached to a given payment
- `appdirect-cli billing resource-other-read-payments-get`  -  List all payments matching the input filters
- `appdirect-cli billing resource-other-read-purchase-order-get`  -  Retrieve a purchase order. Rate limit: Bucket size is 20 requests, with a leak rate of 4 requests per second.
- `appdirect-cli billing resource-other-read-purchase-orders-get`  -  List all purchase orders
- `appdirect-cli billing resource-other-read-subscription-purchase-orders-get`  -  List all the purchase orders for the given subscription
- `appdirect-cli billing resource-other-read-user-company-product-context-get`  -  Retrieve a product context for a supplied user and a company they belong to
- `appdirect-cli billing resource-other-read-user-invoices-get`  -  List all of the invoices for the given user
- `appdirect-cli billing resource-other-read-user-payment-instruments-get`  -  List all of the payment instruments for the given user
- `appdirect-cli billing resource-other-read-user-payments-get`  -  List all of the payments for the given user
- `appdirect-cli billing resource-other-read-user-product-context-get`  -  Retrieve a product context for the current user
- `appdirect-cli billing resource-other-read-user-purchase-orders-get`  -  List all of the purchase orders for the given user
- `appdirect-cli billing resource-other-read-user-subscriptions-get`  -  List all of the subscriptions for the given user
- `appdirect-cli billing resource-other-update-order-configuration-put`  -  Update purchase order configuration details
- `appdirect-cli billing resource-other-update-payment-instrument-put`  -  Update the payment instrument for the given user and company using the provided data
- `appdirect-cli billing resource-other-update-subscription-patch`  -  Use this request to manage subscription lifecycles (suspend and activate subscriptions), update external IDs
- `appdirect-cli billing resource-subscription-cancel-addon-instance-delete`  -  Delete the given add-on instance on the given subscription
- `appdirect-cli billing resource-subscription-cancel-subscription-delete`  -  Requests cancellation of the given subscription
- `appdirect-cli billing resource-subscription-change-addon-instance-put`  -  Update the given add-on instance on the given subscription using the provided data
- `appdirect-cli billing resource-subscription-create-addon-instance-post`  -  Create an add-on instance on the given subscription using the given data
- `appdirect-cli billing resource-subscription-preview-cancel-subscription-get`  -  Preview a subscription cancellation for the given subscription ID
- `appdirect-cli billing resource-subscription-preview-change-addon-instance-put`  -  Preview change of an addon instance for a subscription
- `appdirect-cli billing resource-subscription-preview-create-addon-instance-post`  -  Preview creation of an addon instance for a subscription
- `appdirect-cli billing resource-subscription-read-addon-instances-get`  -  Read addon instances for a subscription
- `appdirect-cli billing resource-subscription-read-subscription-get`  -  Retrieve the subscription for the given subscription ID
- `appdirect-cli billing resource-subscription-read-subscription-invoices-get`  -  List all of the invoices for the given subscription
- `appdirect-cli billing resource-subscription-read-subscription-payments-get`  -  List all the payments for the given subscription
- `appdirect-cli billing resource-subscription-read-subscriptions-get`  -  The list may be filtered using the optional filter parameters

**channel**  -  Manage channel

- `appdirect-cli channel create-company-group`  -  Creates a segment folder
- `appdirect-cli channel create-dynamic-segment`  -  Creates a dynamic segment.
- `appdirect-cli channel create-segment`  -  Creates a manual segment.
- `appdirect-cli channel delete-and-add-companies`  -  Creates or removes associations between segments and companies
- `appdirect-cli channel delete-company-group`  -  Deletes a segment folder, all associated segments, and all product associations
- `appdirect-cli channel get-available-and-associated-companies`  -  Returns a paginated list of all marketplace companies and indicates whether they are associated with the segment
- `appdirect-cli channel get-company-group-segments`  -  Returns a paginated list of segments
- `appdirect-cli channel get-company-groups`  -  Returns a paginated list of segment folders for the marketplace
- `appdirect-cli channel read-dynamic-segment`  -  Reads a dynamic segment
- `appdirect-cli channel read-filter-parameter`  -  Returns a list of parameters for the dynamic filter
- `appdirect-cli channel remove-segment`  -  Deletes a segment from a segment folder
- `appdirect-cli channel resource-other-create-discount-post`  -  Creates a discount with the provided data.
- `appdirect-cli channel resource-other-currency-exchange-rate-patch`  -  Deactivate a Currency Exchange Rate. To stop supporting currency exchange on your marketplace, deactivate a rate.
- `appdirect-cli channel resource-other-currency-exchange-rates-get`  -  Retrieve all of the current and historical exchange rates set on the marketplace.
- `appdirect-cli channel resource-other-currency-exchange-rates-post`  -  Define a new exchange rate to be used when custom metered usage is reported to your marketplace by a Developer in a
- `appdirect-cli channel resource-other-delete-discount-delete`  -  Deletes the discount with the specified discount ID or UUID.
- `appdirect-cli channel resource-other-read-currency-exchange-rate-get`  -  Retrieve the details of a specific Currency Exchange Rate.
- `appdirect-cli channel resource-other-read-discount-get`  -  Retrieves the discount for the given discount ID or UUID.
- `appdirect-cli channel resource-other-read-discounts-get`  -  Lists all available discounts. The parameters can be used to filter the results.
- `appdirect-cli channel resource-other-read-event-by-id-get`  -  This call returns all details for a specific event.
- `appdirect-cli channel resource-other-read-events-get`  -  This call lists all events on your marketplace.
- `appdirect-cli channel resource-other-update-discount-put`  -  Updates the specified discount with the provided data.
- `appdirect-cli channel resource-product-read-products-get`  -  Retrieve all products in the production catalog
- `appdirect-cli channel resource-product-read-staging-catalog-get`  -  This call lists all products in the Staging Catalog of your marketplace.
- `appdirect-cli channel resource-settings-api-get-get`  -  Use the GET method to list channel settings for a specific channel.
- `appdirect-cli channel resource-settings-api-update-patch`  -  Use the PATCH method to update one or more channel settings.
- `appdirect-cli channel test-dynamic-segment`  -  Determines whether the specified user matches the specified dynamic segment
- `appdirect-cli channel update-company-group`  -  Updates a segment folder
- `appdirect-cli channel update-dynamic-segment`  -  Updates a dynamic segment
- `appdirect-cli channel update-segment`  -  Updates a manual segment

**checkout**  -  Manage checkout

- `appdirect-cli checkout create-shopping-cart`  -  Creates a shopping cart
- `appdirect-cli checkout create-shopping-cart-fee`  -  Creates a fee and applies it to the specified cart
- `appdirect-cli checkout create-shopping-cart-item`  -  Adds the specified item to the shopping cart
- `appdirect-cli checkout create-shopping-cart-validation-by-shopping-cart-id`  -  Validates and retrieves the validation summary of the specified shopping cart
- `appdirect-cli checkout delete-discount-by-shopping-cart-id`  -  Deletes a discount by code from shopping cart items.
- `appdirect-cli checkout delete-shopping-cart`  -  Deletes the specified shopping cart
- `appdirect-cli checkout delete-shopping-cart-accredited-agents`  -  Removes the accredited agent from the specified cart
- `appdirect-cli checkout delete-shopping-cart-fee`  -  Removes fees from the specified cart
- `appdirect-cli checkout delete-shopping-cart-item`  -  Removes the specified item from the shopping cart
- `appdirect-cli checkout get-item-details`  -  Retrieves product details such as branding, pricing, and so on.
- `appdirect-cli checkout get-pre-auth`  -  Retrieves the payment pre-authorization for the specified cart.
- `appdirect-cli checkout get-pricing-summary`  -  Retrieves a pricing summary for the cart.
- `appdirect-cli checkout get-pricing-summary-by-shopping-cart-id`  -  Retrieves a pricing summary for the cart.
- `appdirect-cli checkout get-shopping-cart`  -  Retrieves the specified shopping cart
- `appdirect-cli checkout get-shopping-cart-associations`  -  Returns the associations included in the shopping cart
- `appdirect-cli checkout get-shopping-cart-item`  -  Returns the specified shopping cart item
- `appdirect-cli checkout get-shopping-cart-validation-by-shopping-cart-id`  -  Retrieves the validation summary for the specified shopping cart, but does not validate the cart.
- `appdirect-cli checkout list-shopping-cart-items`  -  Returns all items included in the shopping cart
- `appdirect-cli checkout list-shopping-carts`  -  Returns all ACTIVE and FINISHED shopping carts
- `appdirect-cli checkout notify-locked-carts`  -  Finds all locked carts in ACTIVE status and sends approval request notifications for each one.
- `appdirect-cli checkout preview-shopping-cart`  -  Returns a preview of the shopping cart with pricing summary information and a list of errors, if any
- `appdirect-cli checkout preview-shopping-cart-by-id`  -  Previews the specified, persisted shopping cart
- `appdirect-cli checkout update-shopping-cart`  -  Updates the specified shopping cart
- `appdirect-cli checkout update-shopping-cart-accredited-agents`  -  Associates an accredited Agent with the specified cart
- `appdirect-cli checkout update-shopping-cart-item`  -  Updates the specified shopping cart item
- `appdirect-cli checkout validate-shopping-cart`  -  Validates and retrieves the validation summary of the specified shopping cart payload

**integration**  -  Manage integration

- `appdirect-cli integration resource-other-bill-usage-post`  -  Submit usage data to be billed for the given account identifier
- `appdirect-cli integration resource-other-bill-usage-v2-get`  -  Retrieve whether the metered usage events submitted with a Billing Usage V2 request were successfully processed.
- `appdirect-cli integration resource-other-bill-usage-v2-post`  -  Submit preconfigured, custom, or volume-priced metered usage data to be billed for a customer account.
- `appdirect-cli integration resource-other-domain-verification-status-post`  -  Status verification

**lead**  -  Manage lead

- `appdirect-cli lead resource-app-reseller-api-activities-get`  -  Retrieves a list of all lead activities visible for a given context.
- `appdirect-cli lead resource-app-reseller-api-assign-post`  -  Assigns a lead to a Reseller company.
- `appdirect-cli lead resource-app-reseller-api-associate-post`  -  Associates a lead with an existing marketplace user and company
- `appdirect-cli lead resource-app-reseller-api-convert-approval-post`  -  Approves the conversion request made by a Reseller company
- `appdirect-cli lead resource-app-reseller-api-convert-post`  -  Converts the lead to a marketplace company
- `appdirect-cli lead resource-app-reseller-api-convert-request-post`  -  Request permission to convert the lead to a marketplace company
- `appdirect-cli lead resource-app-reseller-api-create-post`  -  Creates a manual, company profile or product profile lead. Manual leads are visible to only those who created them.
- `appdirect-cli lead resource-app-reseller-api-delete-delete`  -  Deletes or disqualifies a lead (depending on type and assignment)
- `appdirect-cli lead resource-app-reseller-api-get-all-get`  -  Retrieves a list of all leads visible for a given context. You can filter the list using optional query parameters.
- `appdirect-cli lead resource-app-reseller-api-get-get`  -  Retrieves a single lead and its details
- `appdirect-cli lead resource-app-reseller-api-update-patch`  -  Updates the contact information or notes from a lead

**marketplace**  -  Manage marketplace

- `appdirect-cli marketplace resource-answer-create-product-question-answer-post`  -  This call creates an answer to a product question submitted on your marketplace.
- `appdirect-cli marketplace resource-answer-delete-product-question-answer-delete`  -  This call deletes an answer from your marketplace.
- `appdirect-cli marketplace resource-answer-read-product-question-answer-get`  -  This call returns all answer details for a specific product.
- `appdirect-cli marketplace resource-answer-read-product-question-answers-get`  -  List answers of a given question on an product
- `appdirect-cli marketplace resource-answer-update-product-question-answer-put`  -  This call updates an existing answer on your marketplace.
- `appdirect-cli marketplace resource-bundle-read-bundle-get`  -  This call returns all details about a specific bundle.
- `appdirect-cli marketplace resource-bundle-read-bundle-status-get`  -  This call returns the current state of the bundle.
- `appdirect-cli marketplace resource-bundle-read-bundles-get`  -  This call lists all bundles on the marketplace.
- `appdirect-cli marketplace resource-comment-create-product-review-comment-post`  -  This call creates a new review comment on your marketplace.
- `appdirect-cli marketplace resource-comment-delete-product-review-comment-delete`  -  This call deletes a review comment from your marketplace.
- `appdirect-cli marketplace resource-comment-read-product-review-comment-get`  -  This call returns all the comment details from a specific product.
- `appdirect-cli marketplace resource-comment-read-product-review-comments-get`  -  This call lists all review comments on your marketplace.
- `appdirect-cli marketplace resource-comment-update-product-review-comment-put`  -  This call updates a product comment on your marketplace.
- `appdirect-cli marketplace resource-edition-read-edition-get`  -  This call returns all details related to a specific product edition
- `appdirect-cli marketplace resource-navigation-read-navigator-get`  -  This call lists all product groups (attributes, categories, and customer tags) that are used on your marketplace.
- `appdirect-cli marketplace resource-payment-plan-read-payment-plan-get`  -  Read payment plan information
- `appdirect-cli marketplace resource-payment-plan-read-payment-plan-id-get`  -  Read payment plan by product edition
- `appdirect-cli marketplace resource-payment-plan-read-payment-plans-get`  -  List payment plans for a given product edition
- `appdirect-cli marketplace resource-product-read-addon-listing-get`  -  Lists add-on products visible on the current marketplace.
- `appdirect-cli marketplace resource-product-read-app-listing-get`  -  This call lists products based on specific parameters such as attribute, category, date, or type.
- `appdirect-cli marketplace resource-product-read-product-get`  -  This request returns all details about a specific product on your marketplace.
- `appdirect-cli marketplace resource-product-read-product-recommendations-get`  -  This call lists all products that other customers have bought in addition to the one specified.
- `appdirect-cli marketplace resource-product-read-status-get`  -  This call returns the status of a product on your marketplace.
- `appdirect-cli marketplace resource-product-read-vendor-products-get`  -  Lists all products offered by the specified vendor.
- `appdirect-cli marketplace resource-question-delete-product-question-delete`  -  This call deletes a product question from your marketplace.
- `appdirect-cli marketplace resource-question-read-product-question-get`  -  This call returns all question details for a specific product.
- `appdirect-cli marketplace resource-question-read-product-questions-get`  -  This call lists all questions listed on your marketplace.
- `appdirect-cli marketplace resource-question-update-product-question-put`  -  This call allows you to update a product question on your marketplace.
- `appdirect-cli marketplace resource-review-create-product-review-post`  -  This call creates a new product review on your marketplace.
- `appdirect-cli marketplace resource-review-delete-product-review-delete`  -  Delete a review This call deletes a product review from your marketplace.
- `appdirect-cli marketplace resource-review-read-product-review-get`  -  This call returns all details for a specific review.
- `appdirect-cli marketplace resource-review-read-product-reviews-get`  -  This call lists all reviews and associated information listed on your marketplace.
- `appdirect-cli marketplace resource-review-update-product-review-put`  -  Update a review This call updates an existing product review on your marketplace.

**marketplace-product**  -  Manage marketplace product

- `appdirect-cli marketplace-product resource-product-active-setting-read-settings-get`  -  Retrieve the settings, sections
- `appdirect-cli marketplace-product resource-product-setting-read-settings-by-ref-id-get`  -  Retrieve product settings, including product information, general settings, product group
- `appdirect-cli marketplace-product resource-product-setting-read-settings-get`  -  Retrieve product settings, including product information, general settings, product group
- `appdirect-cli marketplace-product resource-product-setting-update-settings-by-ref-id-put`  -  Update product settings on a given marketplace by product reference ID
- `appdirect-cli marketplace-product resource-product-setting-update-settings-put`  -  Update product settings for a product on a given marketplace.

**notification**  -  Manage notification

- `appdirect-cli notification resource-default-template-api-create-or-update-default-common-email-template-post`  -  Create or update a default common email template element.
- `appdirect-cli notification resource-default-template-api-create-or-update-default-email-template-post`  -  Create or update default email template. Reserved for super support users.
- `appdirect-cli notification resource-default-template-api-get-default-common-email-template-get`  -  Read default common email element by type.
- `appdirect-cli notification resource-default-template-api-get-default-email-template-get`  -  Read default email template by type. Reserved for super support user.
- `appdirect-cli notification resource-default-template-api-get-default-template-definitions-get`  -  Read default notification templates available for the current channel. Reserved for super support users.
- `appdirect-cli notification resource-template-api-create-or-update-common-email-template-post`  -  This call allows you to create a new template element (for example, a variable) or update an existing one.
- `appdirect-cli notification resource-template-api-create-or-update-email-template-post`  -  This call creates a new email template or updates an existing template.
- `appdirect-cli notification resource-template-api-create-or-update-sms-template-post`  -  This call create a new element or updates an existing SMS template.
- `appdirect-cli notification resource-template-api-get-common-email-template-get`  -  Retrieve common email element by type for the current channel
- `appdirect-cli notification resource-template-api-get-common-templates-definitions-get`  -  List common element definitions that is present in each notification that is sent e.g.
- `appdirect-cli notification resource-template-api-get-email-template-get`  -  This call returns all details from a specific email template type.
- `appdirect-cli notification resource-template-api-get-sms-template-get`  -  This call returns all details for a specific sms template type.
- `appdirect-cli notification resource-template-api-get-template-definitions-get`  -  List notification templates for the current channel
- `appdirect-cli notification resource-template-api-get-template-parameters-get`  -  This call returns all parameter details from a notification template for a specified template type.

**product-uploader-api**  -  Manage product uploader api

- `appdirect-cli product-uploader-api resource-data-uploader-create-catalog-post`  -  Create a new product catalog using a CSV file.
- `appdirect-cli product-uploader-api resource-data-uploader-publish-catalog-post`  -  Publish a product catalog using a CSV file. This endpoint makes the catalog available for use in the marketplace.
- `appdirect-cli product-uploader-api resource-data-uploader-update-catalog-post`  -  Update an existing product catalog using a CSV file.

**products**  -  Manage products

- `appdirect-cli products delete-file`  -  Deletes a specific file resource from a product. Currently supports deletion of PDF files only.
- `appdirect-cli products delete-image`  -  Deletes a specific image resource from a product.
- `appdirect-cli products upload-and-link-image`  -  Uploads an image file and links it to a specific product resource.
- `appdirect-cli products upload-and-link-pdf-file`  -  Uploads a PDF file and links it to a specific product as documentation or resource material.

**reconciliation**  -  Manage reconciliation

- `appdirect-cli reconciliation`  -  Retrieve ledger lines generated for a specific role.

**reporting**  -  Manage reporting

- `appdirect-cli reporting resource-other-delete-report-delete`  -  This call deletes a report from your marketplace
- `appdirect-cli reporting resource-other-read-report-get`  -  This call returns all details from a specific report.
- `appdirect-cli reporting resource-other-read-reports-v1-get`  -  Lists all reports that are automatically generated on your marketplace.
- `appdirect-cli reporting resource-report-download-reports-v2-get`  -  Download an individual report
- `appdirect-cli reporting resource-report-read-reports-v2-get`  -  Lists all reports generated from your marketplace.

**reseller**  -  Manage reseller

- `appdirect-cli reseller read-transfer`  -  Retrieves the summary of a transfer, which includes the date of the transfer, the transfer status, currency
- `appdirect-cli reseller read-transfer-details`  -  Retrieves the ledger lines of a transfer
- `appdirect-cli reseller read-transfers`  -  Retrieves all transfers

**shopping-carts**  -  Operations related to shopping carts



### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
appdirect-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Month-close billing reconciliation

```bash
appdirect-cli reconcile --since 30d --agent --select findings.kind,findings.subscriptionId,findings.companyName
```

Narrow the reconciliation report to just the mismatch type, subscription, and company for fast agent triage.

### Weekly failed-payment chase

```bash
appdirect-cli payments unpaid --since 7d --json
```

Failed, declined, and overdue payments across all companies, sorted by amount and age.

### What changed this week

```bash
appdirect-cli subs changed --since 7d --json
```

Cross-company subscription lifecycle diff - created, cancelled, suspended.

### Pipeline review before forecast call

```bash
appdirect-cli pipeline --group-by status --agent
```

Opportunity counts and ages rolled up by status from the local store.

### Customer snapshot for a support ticket

```bash
appdirect-cli company show 12345678-aaaa-bbbb-cccc-1234567890ab --json
```

Users, subscriptions, invoices, and open opportunities for one company in a single payload.

## Auth Setup

AppDirect uses OAuth 2.0 client-credentials per marketplace. Create an API client in your marketplace settings (Settings > API Clients), then set three environment variables: APPDIRECT_BASE_URL (your marketplace host, e.g. https://marketplace.appdirect.com), APPDIRECT_CLIENT_ID, and APPDIRECT_CLIENT_SECRET. The CLI mints bearer tokens from <base-url>/oauth2/token automatically and re-mints on expiry - no manual token handling. Client-credentials grants are limited to the ROLE_PARTNER and ROLE_PARTNER_READ scopes; user-context endpoints outside those scopes will return 403.

Run `appdirect-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  appdirect-cli account resource-company-create-active-user-post <id> --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
appdirect-cli feedback "the --since flag is inclusive but docs say exclusive"
appdirect-cli feedback --stdin < notes.txt
appdirect-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/appdirect-cli/feedback.jsonl`. They are never POSTed unless `APPDIRECT_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `APPDIRECT_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
appdirect-cli profile save briefing --json
appdirect-cli --profile briefing account resource-company-create-active-user-post <id>
appdirect-cli profile list --json
appdirect-cli profile show briefing
appdirect-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Async Jobs

For endpoints that submit long-running work, the generator detects the submit-then-poll pattern (a `job_id`/`task_id`/`operation_id` field in the response plus a sibling status endpoint) and wires up three extra flags on the submitting command:

| Flag | Purpose |
|------|---------|
| `--wait` | Block until the job reaches a terminal status instead of returning the job ID immediately |
| `--wait-timeout` | Maximum wait duration (default 10m, 0 means no timeout) |
| `--wait-interval` | Initial poll interval (default 2s; grows with exponential backoff up to 30s) |

Use async submission without `--wait` when you want to fire-and-forget; use `--wait` when you want one command to return the finished artifact.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `appdirect-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/commerce/appdirect/cmd/appdirect-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add appdirect-mcp -- appdirect-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which appdirect-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   appdirect-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `appdirect-cli <command> --help`.
