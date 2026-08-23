// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Single source of truth for every Auvik attribute the hand-written commands read.
//
// WHY THIS FILE EXISTS
// The first cut of these commands used invented attribute names (endOfSupport,
// detectedTime, configContents, billableDeviceCount ...). None of them exist in
// Auvik's OpenAPI documents. The unit tests used the SAME invented names in their
// fixtures, so the suite was green while every command returned an empty or false
// answer against real data. Tests validated the bug instead of the API.
//
// Every field below is declared once, bound to the spec type that must declare
// it, and referenced by the commands through `.Field`. `TestAuvikSpecFieldsExist`
// walks the shipped spec.json and fails if any entry is missing, so a name the
// API does not emit cannot survive a build. Fixtures are generated from this same
// registry, which makes the circular-validation failure structurally impossible.

package cli

// specField binds one attribute name to the spec type that declares it.
type specField struct {
	Type  string // key under spec.json ".types"
	Field string // entry under that type's "fields[].name"
}

// specRel binds one JSON:API relationship name to the relationships type that
// declares it.
type specRel struct {
	Type string
	Name string
}

// ---- device (v1 inventory) ----
var (
	fDeviceName      = specField{"deviceAttributes", "deviceName"}
	fDeviceType      = specField{"deviceAttributes", "deviceType"}
	fDeviceMakeModel = specField{"deviceAttributes", "makeModel"}
	fDeviceSerial    = specField{"deviceAttributes", "serialNumber"}
	fDeviceFirmware  = specField{"deviceAttributes", "firmwareVersion"}
	fDeviceSoftware  = specField{"deviceAttributes", "softwareVersion"}
	fDeviceOnline    = specField{"deviceAttributes", "onlineStatus"}
	fDeviceLastSeen  = specField{"deviceAttributes", "lastSeenTime"}
	fDeviceVendor    = specField{"deviceAttributes", "vendorName"}
)

// ---- lifecycle: v1 carries STATUS ENUMS, v2 carries the DATES ----
var (
	fLifecycleLastSupportStatus = specField{"deviceLifecycleAttributes", "lastSupportStatus"}
	fLifecycleSalesAvail        = specField{"deviceLifecycleAttributes", "salesAvailability"}
	fLifecycleSwMaint           = specField{"deviceLifecycleAttributes", "softwareMaintenanceStatus"}
	fLifecycleSecSwMaint        = specField{"deviceLifecycleAttributes", "securitySoftwareMaintenanceStatus"}

	fLifecycleV2LastSupportDate = specField{"deviceLifecycleAttributesV2", "lastSupportDate"}
	fLifecycleV2SalesAvailDate  = specField{"deviceLifecycleAttributesV2", "salesAvailabilityDate"}
	fLifecycleV2MakeModel       = specField{"deviceLifecycleAttributesV2", "makeModel"}
)

// ---- warranty ----
var (
	fWarrantyExpiration     = specField{"deviceWarrantyAttributes", "warrantyExpirationDate"}
	fWarrantyCoverageStatus = specField{"deviceWarrantyAttributes", "warrantyCoverageStatus"}
	fServiceCoverageStatus  = specField{"deviceWarrantyAttributes", "serviceCoverageStatus"}
)

// ---- configuration (metadata only: Auvik does NOT expose config bodies) ----
var (
	fConfigBackupTime = specField{"configAttributes", "backupTime"}
	fConfigIsRunning  = specField{"configAttributes", "isRunning"}
)

// ---- alerts (note: `dismissed` is a BOOL and there is NO dismissal timestamp) ----
var (
	fAlertDetectedOn  = specField{"alertAttributes", "detectedOn"}
	fAlertName        = specField{"alertAttributes", "name"}
	fAlertDescription = specField{"alertAttributes", "description"}
	fAlertSeverity    = specField{"alertAttributes", "severity"}
	fAlertStatus      = specField{"alertAttributes", "status"}
	fAlertDismissed   = specField{"alertAttributes", "dismissed"}
)

// ---- entity audit / note ----
var (
	fAuditDateStarted = specField{"auditAttributes", "dateStarted"}
	fAuditLastActive  = specField{"auditAttributes", "lastActive"}
	fAuditUser        = specField{"auditAttributes", "user"}
	fAuditAction      = specField{"auditAttributes", "action"}
	fAuditCategory    = specField{"auditAttributes", "category"}
	fAuditStatus      = specField{"auditAttributes", "status"}

	fNoteBody           = specField{"noteAttributes", "body"}
	fNoteTitle          = specField{"noteAttributes", "title"}
	fNoteLastModified   = specField{"noteAttributes", "lastModified"}
	fNoteLastModifiedBy = specField{"noteAttributes", "lastModifiedBy"}
	fNoteEntityID       = specField{"noteAttributes", "entityId"}
)

// ---- billing / usage ----
var (
	fUsageDomainPrefix    = specField{"clientUsageAttributes", "domainPrefix"}
	fUsageBillableDays    = specField{"clientUsageAttributes", "billableDays"}
	fUsageMonServers      = specField{"clientUsageAttributes", "monitoredServers"}
	fUsageMonWorkstations = specField{"clientUsageAttributes", "monitoredWorkstations"}
	fUsageMaxServers      = specField{"clientUsageAttributes", "maxMonitoredServers"}
	fUsageMaxWorkstations = specField{"clientUsageAttributes", "maxMonitoredWorkstations"}
)

// ---- discovery status (v2) ----
var (
	fDiscoveryLogin  = specField{"deviceDiscoveryStatusAttributes", "login"}
	fDiscoverySNMP   = specField{"deviceDiscoveryStatusAttributes", "snmp"}
	fDiscoveryVMware = specField{"deviceDiscoveryStatusAttributes", "vmware"}
	fDiscoveryWMI    = specField{"deviceDiscoveryStatusAttributes", "wmi"}

	fDetailDiscoveryStatus = specField{"deviceDetailsAttributes", "discoveryStatus"}
)

// ---- ASM ----
var (
	fASMAppName      = specField{"asmAppAttributes", "name"}
	fASMAppCategory  = specField{"asmAppAttributes", "category"}
	fASMAppDisabled  = specField{"asmAppAttributes", "disabled"}
	fASMUserEmail    = specField{"asmUserAttributes", "email"}
	fASMUserActive   = specField{"asmUserAttributes", "active"}
	fASMUserDisabled = specField{"asmUserAttributes", "disabled"}
	fASMLicenseEmail = specField{"asmLicenseAttributes", "email"}
	fASMLicenseType  = specField{"asmLicenseAttributes", "licenseType"}
	fASMClientName   = specField{"asmClientAttributes", "name"}
)

// ---- relationships ----
var (
	rConfigDevice = specRel{"configRelationships", "device"}
	rConfigTenant = specRel{"configRelationships", "tenant"}
	rAlertEntity  = specRel{"alertRelationships", "entity"}
	rAlertTenant  = specRel{"alertRelationships", "tenant"}
	rNoteTenant   = specRel{"noteRelationships", "tenant"}
	rASMAppClient = specRel{"asmAppRelationships", "client"}
	rASMAppUsers  = specRel{"asmAppRelationships", "users"}
	rASMUserApps  = specRel{"asmUserRelationships", "applications"}
	rUsageDevices = specRel{"clientUsageRelationships", "devices"}
	rUsageClients = specRel{"clientUsageRelationships", "clients"}
)

// auvikSpecFields is every attribute the hand-written commands read.
// TestAuvikSpecFieldsExist validates each against the shipped spec.
var auvikSpecFields = []specField{
	fDeviceName, fDeviceType, fDeviceMakeModel, fDeviceSerial, fDeviceFirmware,
	fDeviceSoftware, fDeviceOnline, fDeviceLastSeen, fDeviceVendor,
	fLifecycleLastSupportStatus, fLifecycleSalesAvail, fLifecycleSwMaint, fLifecycleSecSwMaint,
	fLifecycleV2LastSupportDate, fLifecycleV2SalesAvailDate, fLifecycleV2MakeModel,
	fWarrantyExpiration, fWarrantyCoverageStatus, fServiceCoverageStatus,
	fConfigBackupTime, fConfigIsRunning,
	fAlertDetectedOn, fAlertName, fAlertDescription, fAlertSeverity, fAlertStatus, fAlertDismissed,
	fAuditDateStarted, fAuditLastActive, fAuditUser, fAuditAction, fAuditCategory, fAuditStatus,
	fNoteBody, fNoteTitle, fNoteLastModified, fNoteLastModifiedBy, fNoteEntityID,
	fUsageDomainPrefix, fUsageBillableDays, fUsageMonServers, fUsageMonWorkstations,
	fUsageMaxServers, fUsageMaxWorkstations,
	fDiscoveryLogin, fDiscoverySNMP, fDiscoveryVMware, fDiscoveryWMI, fDetailDiscoveryStatus,
	fASMAppName, fASMAppCategory, fASMAppDisabled,
	fASMUserEmail, fASMUserActive, fASMUserDisabled,
	fASMLicenseEmail, fASMLicenseType, fASMClientName,
}

// auvikSpecRels is every relationship the hand-written commands traverse.
var auvikSpecRels = []specRel{
	rConfigDevice, rConfigTenant, rAlertEntity, rAlertTenant, rNoteTenant,
	rASMAppClient, rASMAppUsers, rASMUserApps, rUsageDevices, rUsageClients,
}

// Lifecycle/warranty status enum values that mean "no longer covered".
// Source: deviceLifecycleAttributes enum [covered available expired securityOnly
// unpublished empty].
func statusMeansExpired(s string) bool {
	switch s {
	case "expired":
		return true
	}
	return false
}

// statusMeansSecurityOnly marks the in-between state worth surfacing separately.
func statusMeansSecurityOnly(s string) bool { return s == "securityOnly" }

// Discovery probe enum values. Source: deviceDiscoveryStatusAttributes enum
// [disabled determining notSupported notAuthorized authorizing authorized privileged].
// "notAuthorized" is the credential failure an operator must act on; the
// transitional states mean Auvik is still working and are reported separately.
func discoveryProbeHealthy(v string) bool {
	switch v {
	case "authorized", "privileged", "notSupported", "disabled", "":
		return true
	}
	return false
}

func discoveryProbeTransitional(v string) bool {
	switch v {
	case "determining", "authorizing":
		return true
	}
	return false
}
