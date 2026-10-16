// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import "strings"

// Hand-wired (handfixes.json: mcp-read-only-allows-gravityzone-reads).
// Every GravityZone method is a JSON-RPC POST, so the generated read-only
// switch (refuse anything that is not GET) refused every read as well. Under
// BITDEFENDER_MCP_READ_ONLY=1, bitdefender_execute also runs the methods
// listed here: methods that only read. The list is explicit, so a method
// added by a later regeneration stays refused until someone adds it here.
// Reads that return or take a credential stay refused: push#getPushEventSettings
// (the webhook Authorization value), licensing#getLicenseInfo (license key),
// companies#getCompanyDetailsByUser (takes a user's password),
// packages#getPackageDetails (package settings, uninstall password),
// packages#getInstallationLinks (links that enroll a machine),
// integrations#getIntegrationDetails (integration connection settings),
// policies#getPolicyDetails (policy settings carry the password-protection and
// power-user password fields) and network#getNetworkInventoryItems (company
// items can carry license keys).
var gravityZoneReadOnlyMethods = map[string]bool{
	"accounts#getAccountDetails":                             true,
	"accounts#getAccountsList":                               true,
	"accounts#getNotificationsSettings":                      true,
	"companies#findCompaniesByName":                          true,
	"companies#getCompanyDetails":                            true,
	"companies#getCustomFieldsDefinitions":                   true,
	"general#getApiKeyDetails":                               true,
	"incidents#getBlocklistItems":                            true,
	"incidents#getCustomRulesList":                           true,
	"incidents#getIncident":                                  true,
	"incidents#getIncidentsByIds":                            true,
	"incidents#getIncidentsList":                             true,
	"incidents#getResponseActionStatus":                      true,
	"incidents#getSimilarEmails":                             true,
	"integrations#getAmazonEC2ExternalIdForCrossAccountRole": true,
	"integrations#getCompanyDetailsByAWSAccountId":           true,
	"integrations#getConfiguredIntegrations":                 true,
	"integrations#getHourlyUsageForAmazonEC2Instances":       true,
	"investigation#getInvestigationFileUrl":                  true,
	"licensing#getMonthlyUsage":                              true,
	"licensing#getMonthlyUsagePerProductType":                true,
	"maintenanceWindows#getMaintenanceWindowDetails":         true,
	"maintenanceWindows#getMaintenanceWindowsList":           true,
	"maintenanceWindows#getManuallyApprovedPatches":          true,
	"network#getCompaniesList":                               true,
	"network#getCompanyFoldersList":                          true,
	"network#getCustomGroupsList":                            true,
	"network#getEndpointsList":                               true,
	"network#getEndpointTags":                                true,
	"network#getIntegrators":                                 true,
	"network#getManagedEndpointDetails":                      true,
	"network#getRootContainers":                              true,
	"network#getScanTasksList":                               true,
	"network#getTaskStatus":                                  true,
	"packages#getPackagesList":                               true,
	"patchManagement#getInstalledPatches":                    true,
	"patchManagement#getMissingPatches":                      true,
	"phasr#getAllCompanyIdentities":                          true,
	"phasr#getAllCompanyResources":                           true,
	"phasr#getMonitoredRuleData":                             true,
	"phasr#getMonitoredRules":                                true,
	"phasr#getPhasrRecommendations":                          true,
	"phasr#getRecommendationProfiles":                        true,
	"policies#getPoliciesList":                               true,
	"push#getPushEventStats":                                 true,
	"quarantine/{service}#getQuarantineItemsList":            true,
	"reports#getDownloadLinks":                               true,
	"reports#getReportsList":                                 true,
}

// gravityZoneReadOnlyAllows reports whether a registry path template
// (/v1.x/jsonrpc/<area>#<method>) names a method listed above. It reads the
// endpoint's registered template, never the resolved request path, so a
// path parameter value cannot change the method it names.
func gravityZoneReadOnlyAllows(pathTemplate string) bool {
	_, rest, ok := strings.Cut(pathTemplate, "/jsonrpc/")
	if !ok || !strings.HasPrefix(pathTemplate, "/v1.") {
		return false
	}
	return gravityZoneReadOnlyMethods[rest]
}
