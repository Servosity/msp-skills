// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The masked-placeholder tests, on the built binaries (handfixes.json
// credkit-wiring). VSPC answers a read of a stored credential with a mask in
// place of the secret ("password": "••••••", also "******" and runs of U+FFFD).
// A mask is no credential: holding it withheld the backup policies and, through
// the kit's matcher, the default sync's agent-jobs partition with them. A real
// value under the same names (GET
// /infrastructure/managementAgents/{uid}/credentials answers one) stays held
// before any sink.

import (
	"encoding/base64"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// linuxPoliciesExample is the VSPC 3.6.2 OpenAPI example of GET
// /configuration/backupPolicies/linux: three masked passwords.
const linuxPoliciesExample = `{"data":[{"instanceUid":"a594d2f6-217f-4a7a-9859-d24fa39b76ce","id":13,"organizationUid":"eaf4034e-f4a7-4f6d-8de4-85764963887c","name":"LServerVolumeCloud","description":"Linux server volume job","createSubtenants":true,"unlimitedSubtenantQuota":false,"repositoryQuotaGB":1,"operationMode":"Server","jobConfiguration":{"backupSource":{"backupMode":"Volume","volumeLevelOptions":{"volumes":[{"volumeType":"MountPoint","path":"/cafe"},{"volumeType":"Device","path":"/work"}]},"fileLevelOptions":null},"backupTarget":{"targetType":"CloudRepository","localPath":null,"sharedFolder":null,"backupRepository":null,"enableDeletedFilesRetention":false,"removeDeletedItemsDataAfter":30},"backupStorage":{"compressionLevel":"Optimal","blockSize":"Local1MB","encryptionEnabled":false,"password":null,"passwordHint":null,"isSnapshotRequired":true},"indexingSettings":{"indexingType":"ExceptSpecifiedFolders","includedFolders":null,"excludedFolders":["home","work","smthelse"]},"scriptSettings":{"enabled":true,"preJobScript":{"fileName":"prejob_pre.sh","content":null},"postJobScript":{"fileName":"postjob_post.sh","content":null},"preFreezeScript":null,"postThawScript":null},"retentionSettings":{"restorePointsCount":7,"retentionDays":7},"scheduleSettings":{"scheduleType":"NotScheduled","dailyScheduleSettings":null,"monthlyScheduleSettings":null,"periodicallyScheduleSettings":null,"activeFullSettings":null,"retrySettings":{"enabled":false,"retryTimes":3,"waitTimeoutMinutes":10},"backupHealthCheckScheduleSettings":null,"syntheticFullSettings":null},"applicationAwareProcessingSettings":{"oracleAapSettings":{"processingType":"TryProcess","credentials":{"username":"root1","password":"••••••"},"truncationConfig":{"truncationMode":"TruncateDisabled","sizeGB":10,"lifeTimeHours":24},"useOracleCredentials":true},"mySqlAapSettings":{"processingType":"RequireSuccess","credentials":{"username":"root2","password":"••••••"},"authType":"MySQLPassword","passwordFilePath":""},"postgreSqlAapSettings":{"processingType":"TryProcess","credentials":{"username":"root3","password":"••••••"},"authType":"PSQLPassword"}},"gfsRetentionSettings":null},"type":"Provider","accessMode":"Public","createdBy":"My Company","modifiedDate":"2025-07-17T20:29:40.7669167-06:00","companies":[],"agents":[],"locations":[]},{"instanceUid":"2fb7b81d-05d2-4deb-b5b1-a9bcd66336f6","id":4,"organizationUid":"875d2322-7e7b-4b74-a354-1744c2a8c5c5","name":"Linux server - Entire computer","description":"This policy should be used if you want to create a backup of the entire Linux computer image. When you restore data from such backup, you will be able to recover the entire computer image as well as data on specific computer volumes: files, folders, application data and so on.","createSubtenants":true,"unlimitedSubtenantQuota":true,"repositoryQuotaGB":null,"operationMode":"Server","jobConfiguration":{"backupSource":{"backupMode":"EntireComputer","volumeLevelOptions":null,"fileLevelOptions":null},"backupTarget":{"targetType":"CloudRepository","localPath":null,"sharedFolder":null,"backupRepository":null,"enableDeletedFilesRetention":false,"removeDeletedItemsDataAfter":30},"backupStorage":{"compressionLevel":"Optimal","blockSize":"Local1MB","encryptionEnabled":false,"password":null,"passwordHint":null,"isSnapshotRequired":true},"indexingSettings":{"indexingType":"None","includedFolders":null,"excludedFolders":["/cdrom","/dev","/media","/mnt","/proc","/tmp","/lost+found"]},"scriptSettings":{"enabled":false,"preJobScript":null,"postJobScript":null,"preFreezeScript":null,"postThawScript":null},"retentionSettings":{"restorePointsCount":7,"retentionDays":7},"scheduleSettings":{"scheduleType":"Daily","dailyScheduleSettings":{"time":"00:30","dailyMode":"Everyday","specificDays":null},"monthlyScheduleSettings":null,"periodicallyScheduleSettings":null,"activeFullSettings":{"scheduleType":"NotScheduled","monthly":null,"weeklyOnDays":null},"retrySettings":{"enabled":true,"retryTimes":3,"waitTimeoutMinutes":10},"backupHealthCheckScheduleSettings":null,"syntheticFullSettings":null},"applicationAwareProcessingSettings":{"oracleAapSettings":{"processingType":"DisableProcess","credentials":null,"truncationConfig":{"truncationMode":"TruncateDisabled","sizeGB":10,"lifeTimeHours":24},"useOracleCredentials":true},"mySqlAapSettings":{"processingType":"DisableProcess","credentials":null,"authType":"MySQLPassword","passwordFilePath":null},"postgreSqlAapSettings":{"processingType":"DisableProcess","credentials":null,"authType":"PSQLPassword"}},"gfsRetentionSettings":null},"type":"Predefined","accessMode":"Public","createdBy":"VAC Company","modifiedDate":"2025-07-17T18:45:19.177-06:00","companies":["4d5d6711-053a-4fd6-8cfb-8c802b5e0af2"],"agents":["9cdccf69-8e48-47e3-8f64-dc780ce11684","78c322bc-b70d-4e75-a14d-bad67a087eff"],"locations":["bea50fe4-9658-44cf-bd28-015f1bf58d21"]}],"meta":{"pagingInfo":{"total":2,"count":2,"offset":0}}}`

// agentJobsExample is the VSPC 3.6.2 OpenAPI example of GET
// /infrastructure/backupAgents/jobs (the default sync's infrastructure
// partition), verbatim. Its typed INSERT was withheld while "••••••" was held,
// and a job name carries "Personal", which a held "******" matched.
const agentJobsExample = `{"data":[{"instanceUid":"70c7fc65-6128-675c-b53b-f0c17762e6f6","originalUid":"bffded0b-7c79-494c-a151-bb200f3fc4f5","backupAgentUid":"861d3b42-f46f-b0c7-4f6f-4e8c507e7b5c","organizationUid":"2da90254-8fe3-4df5-94c3-b68420fbfe23","name":"AgentJob1","description":"Created by VSPC1\\Administrator at 7/15/2019 3:12 PM.","configUid":"e0e29206-2860-49de-974b-781d6c2fedb2","systemType":"Windows","backupPolicyUid":null,"backupPolicyName":null,"backupPolicyAssignStatus":null,"backupPolicyFailureMessage":null,"status":"Failed","operationMode":"Workstation","destination":"C:\\backups\\","restorePoints":2,"lastRun":"2025-07-26T19:34:05.716-06:00","lastEndTime":"2025-07-26T19:35:14.503-06:00","lastDuration":68,"nextRun":null,"avgDuration":68,"backupMode":"File","targetType":"LocalFolder","isEnabled":false,"scheduleType":"Daily","scheduleDisplayName":"Disabled","lastModifiedDate":null,"lastModifiedBy":null,"failureMessage":"Error: Operation was canceled by user WORKGROUP\\SYSTEM","backedUpSize":392183808,"freeSpace":79857876992},{"instanceUid":"7f9a42de-bf49-67c3-8269-c36bfa334585","originalUid":"46a3e465-de0c-4452-953f-4e20daaf37f1","backupAgentUid":"861d3b42-f46f-b0c7-4f6f-4e8c507e7b5c","organizationUid":"2da90254-8fe3-4df5-94c3-b68420fbfe23","name":"Windows workstation - Personal r2vbr","description":"This policy processes user profile folder including all user settings and data.","configUid":"4610eb17-b41c-4cc5-81d1-938ee6d89b7a","systemType":"Windows","backupPolicyUid":"db3fc1fd-e817-4856-82ce-a7d348d88a03","backupPolicyName":"Windows workstation - Personal files","backupPolicyAssignStatus":"Assigned","backupPolicyFailureMessage":null,"status":"None","operationMode":"Workstation","destination":"vspc1.tech.local","restorePoints":0,"lastRun":null,"lastEndTime":null,"lastDuration":null,"nextRun":null,"avgDuration":null,"backupMode":"File","targetType":"CloudRepository","isEnabled":false,"scheduleType":"Daily","scheduleDisplayName":"Disabled","lastModifiedDate":"2025-07-26T18:57:19.6334268-06:00","lastModifiedBy":"VSPC1\\Administrator","failureMessage":null,"backedUpSize":null,"freeSpace":null},{"instanceUid":"725a706d-b44a-6e57-b004-f0dc8ba3ce3b","originalUid":"35b27351-9fde-4216-bc6b-f19683784b24","backupAgentUid":"861d3b42-f46f-b0c7-4f6f-4e8c507e7b5c","organizationUid":"2da90254-8fe3-4df5-94c3-b68420fbfe23","name":"Windows server - Entire computer_r2vbr","description":"This policy should be used if you want to create a backup of the entire computer image. When you restore data from such backup, you will be able to recover the entire computer image as well as data on specific computer volumes: files, folders, application data and so on.","configUid":"9f3bf058-5f78-401b-8067-9b1ede504759","systemType":"Windows","backupPolicyUid":"aec5f032-be73-48a9-a4bb-908848fe61ff","backupPolicyName":"Windows server - Entire computer","backupPolicyAssignStatus":"Assigned","backupPolicyFailureMessage":null,"status":"None","operationMode":"Server","destination":"vspc1.tech.local","restorePoints":0,"lastRun":null,"lastEndTime":null,"lastDuration":null,"nextRun":null,"avgDuration":null,"backupMode":"EntireComputer","targetType":"CloudRepository","isEnabled":false,"scheduleType":"Daily","scheduleDisplayName":"Disabled","lastModifiedDate":"2025-07-26T18:59:18.3197959-06:00","lastModifiedBy":"VSPC1\\Administrator","failureMessage":null,"backedUpSize":null,"freeSpace":null},{"instanceUid":"98a2d3ef-6ad9-68bd-b2d9-3d121940e30f","originalUid":"5bf80578-ed55-43a3-b866-8919b1cc1ce8","backupAgentUid":"861d3b42-f46f-b0c7-4f6f-4e8c507e7b5c","organizationUid":"2da90254-8fe3-4df5-94c3-b68420fbfe23","name":"WServerVolumeCloud_rlab2vbr","description":"temp description","configUid":"37bf043e-a188-4f30-bab8-4efcd5bdaa0d","systemType":"Windows","backupPolicyUid":"ab9fc7e7-b809-44b5-a82b-e1ead4d73094","backupPolicyName":"WServerVolumeCloud","backupPolicyAssignStatus":"Assigned","backupPolicyFailureMessage":null,"status":"None","operationMode":"Server","destination":"vspc1.tech.local","restorePoints":0,"lastRun":null,"lastEndTime":null,"lastDuration":null,"nextRun":null,"avgDuration":null,"backupMode":"Volume","targetType":"CloudRepository","isEnabled":false,"scheduleType":"Daily","scheduleDisplayName":"Disabled","lastModifiedDate":"2025-07-26T19:25:46.4736178-06:00","lastModifiedBy":"ResellerAdministrator","failureMessage":null,"backedUpSize":null,"freeSpace":null}],"meta":{"pagingInfo":{"total":4,"count":4,"offset":0}}}`

const emptyPage = `{"data":[],"meta":{"pagingInfo":{"total":0,"count":0,"offset":0}}}`

// windowsPoliciesExample, linuxDiscoveryRulesExample and
// guestOSCredentialsExample are the VSPC 3.6.2 OpenAPI examples of GET
// /configuration/backupPolicies/windows, /discovery/rules/linux (masked
// rootPassword and sshPrivateKey too) and
// /infrastructure/sites/publicCloud/guestOsCredentials (a run of U+FFFD),
// verbatim. All four mask-bearing lists are partitions of the default sync.
const windowsPoliciesExample = `{"data":[{"instanceUid":"5ab34bcd-efa7-4a4f-864b-2141f2e4a0cf","id":11,"organizationUid":"83baba26-a869-439c-8491-c0d6dd71fb48","name":"WindowSchedule","description":"","createSubtenants":true,"createSubFolders":false,"unlimitedSubtenantQuota":false,"repositoryQuotaGb":1,"operationMode":"Server","jobConfiguration":{"backupSource":{"backupMode":"FilesFolders","computerLevelOptions":null,"volumeLevelOptions":null,"fileLevelOptions":{"includeDirectories":null,"excludeDirectories":null,"inclusionMasks":null,"exclusionMasks":null,"osfilesIncluded":true,"excludeOneDriveFolders":true,"personalFilesIncluded":true,"personalFilesAdvancedSettings":{"inclusions":["Desktop"],"excludeNetworkAccount":false}}},"backupTarget":{"targetType":"SharedFolder","localPath":null,"sharedFolder":{"path":"\\\\vbr\\AZShareSMB01","credentials":{"username":"administrator","password":"••••••"},"backupCacheSettings":null},"backupRepository":null,"cloudRepository":null},"serverModeSettings":{"retentionSettings":{"retentionMode":"Days","retentionCount":1},"scheduleSetting":{"scheduleType":"Continuously","dailyScheduleSettings":null,"monthlyScheduleSettings":null,"periodicalScheduleSettings":null,"continuousScheduleSettings":{"backupWindowSettings":{"scheduleWindow":[{"day":"Sunday","hours":[0,1,2,3,4,5,6,22,23]},{"day":"Monday","hours":[]},{"day":"Tuesday","hours":[]},{"day":"Wednesday","hours":[]},{"day":"Thursday","hours":[]},{"day":"Friday","hours":[]},{"day":"Saturday","hours":[]}],"shiftForMinutes":30}},"retrySettings":{"enabled":true,"retryTimes":3,"waitTimeoutMinutes":10},"backupWindow":[{"day":"Tuesday","hours":[0,1,2,3,4,5,6,22,23]},{"day":"Sunday","hours":[]},{"day":"Monday","hours":[]},{"day":"Wednesday","hours":[]},{"day":"Thursday","hours":[]},{"day":"Friday","hours":[]},{"day":"Saturday","hours":[]}]},"indexingSettings":null,"applicationAwareProcessingSettings":{"enabled":true,"transactionLogProcessingMode":"ProcessTransactionLogsWithJob","sqlServerTransactionLogHandlingSettings":{"credentials":{"username":"usernameTL","password":null},"logsProcessingMode":"TruncateLogs","periodicallyBackupSetting":null},"oracleTransactionLogHandlingSettings":{"credentials":{"accountType":"Oracle","username":"usernameORL","password":"••••••"},"archivedLogsRetentionMode":"DeleteLogsOlderThanHours","backupLifeTimeHours":60,"backupSizeThresholdGb":10,"backupLogsPeriodically":true,"periodicallyBackupSetting":{"backupLogsEveryMin":480,"backupRetentionMode":"KeepLastDays","keepBackupsOnlyLastDays":60}},"sharePointAccountSettings":null,"scriptSettings":{"mode":"IgnoreErrors","preFreezeScript":{"fileName":"prefreeze_Script1.bat","content":null},"postThawScript":{"fileName":"postthaw_Script2.exe","content":null},"credentials":{"username":"usernameS","password":"••••••"}}}},"workstationModeSettings":null,"advancedSettings":{"backupStorage":{"compressionLevel":"Dedupe","storageOptimization":"Lan512KB","encryptionEnabled":true,"password":"••••••","passwordHint":"passwordhint"},"scheduleSettings":{"syntheticFullSettings":null,"activeFullSettings":null},"maintenanceSettings":{"backupHealthCheckSettings":null,"fullBackupFileMaintenanceSettings":{"enableDeletedFilesRetention":true,"removeDeletedItemsDataAfter":30,"defragmentAndCompactFullBackupFileSettings":null},"fullHealthCheck":false}},"gfsRetentionSettings":null},"type":"Provider","accessMode":"Private","createdBy":"My Company","modifiedDate":"2025-07-25T18:31:02.4755583-06:00","companies":[],"agents":[],"locations":[]},{"instanceUid":"a0d1d30d-7db4-4401-898a-c0a45a80de0e","id":2,"organizationUid":"b64d7b8e-49e8-450d-b4f6-60b4ae384b6b","name":"Windows server - Entire computer","description":"This policy should be used if you want to create a backup of the entire computer image. When you restore data from such backup, you will be able to recover the entire computer image as well as data on specific computer volumes: files, folders, application data and so on.","createSubtenants":true,"createSubFolders":false,"unlimitedSubtenantQuota":true,"repositoryQuotaGb":null,"operationMode":"Server","jobConfiguration":{"backupSource":{"backupMode":"EntireComputer","computerLevelOptions":{"includeUsbDrives":false},"volumeLevelOptions":null,"fileLevelOptions":null},"backupTarget":{"targetType":"CloudRepository","localPath":null,"sharedFolder":null,"backupRepository":null,"cloudRepository":null},"serverModeSettings":{"retentionSettings":{"retentionMode":"RestorePoints","retentionCount":7},"scheduleSetting":{"scheduleType":"Daily","dailyScheduleSettings":{"time":"00:30","dailyMode":"Everyday","specificDays":null},"monthlyScheduleSettings":null,"periodicalScheduleSettings":null,"continuousScheduleSettings":null,"retrySettings":{"enabled":true,"retryTimes":3,"waitTimeoutMinutes":10},"backupWindow":null},"indexingSettings":null,"applicationAwareProcessingSettings":{"enabled":true,"transactionLogProcessingMode":"ProcessTransactionLogsWithJob","sqlServerTransactionLogHandlingSettings":{"credentials":null,"logsProcessingMode":"TruncateLogs","periodicallyBackupSetting":null},"oracleTransactionLogHandlingSettings":{"credentials":null,"archivedLogsRetentionMode":"DoNotDeleteArchivedLogs","backupLifeTimeHours":24,"backupSizeThresholdGb":10,"backupLogsPeriodically":false,"periodicallyBackupSetting":null},"sharePointAccountSettings":null,"scriptSettings":{"mode":"Disabled","preFreezeScript":null,"postThawScript":null,"credentials":null}}},"workstationModeSettings":null,"advancedSettings":{"backupStorage":{"compressionLevel":"Optimal","storageOptimization":"Lan512KB","encryptionEnabled":false,"password":null,"passwordHint":null},"scheduleSettings":{"syntheticFullSettings":null,"activeFullSettings":null},"maintenanceSettings":{"backupHealthCheckSettings":null,"fullBackupFileMaintenanceSettings":{"enableDeletedFilesRetention":false,"removeDeletedItemsDataAfter":30,"defragmentAndCompactFullBackupFileSettings":null},"fullHealthCheck":false}},"gfsRetentionSettings":null},"type":"Predefined","accessMode":"Public","createdBy":"VAC Company","modifiedDate":"2025-07-25T16:57:25.113-06:00","companies":["1451f7d7-9f3a-439e-9f42-09f2428a3a0d","a5a7232a-decf-4002-8e4b-66665dc673c5"],"agents":["8894c07b-8b53-4d0b-8f04-6de199c583a8","3eab5715-7a67-4485-974b-8a05b0ac47c5"],"locations":["510e7b96-87b3-48c3-9547-9aacd87f34a9","ffb77a79-5925-4426-b2c6-c076bb384df4"]}],"meta":{"pagingInfo":{"total":2,"count":2,"offset":0}}}`

const linuxDiscoveryRulesExample = `{"data":[{"instanceUid":"796e1b6d-c766-4c17-bf55-fe6bb15fb171","method":"NetworkBased","credentials":[{"instanceUid":"00000008-0000-0000-0000-000000000000","username":"root","password":"••••••","priority":0,"description":"","sshPort":22,"elevateAccountPrivileges":false,"addAccountToSudoersFile":false,"useSuIfsudoFails":false,"rootPassword":null,"sshPrivateKey":null,"passphrase":null,"type":"LinuxBased"}],"deploymentSettings":{"isEnabled":false,"backupPolicyUid":null,"setReadOnlyAccess":true},"_embedded":{"discoveryRule":{"instanceUid":"796e1b6d-c766-4c17-bf55-fe6bb15fb171","name":"Scheduled Linux Network Rule","masterAgentUid":"da463bea-d186-4a14-b809-4aba366b03e8","locationUid":"757dd2a9-b65c-483a-823a-fbfa77cb911f","companyUid":"61204df8-9cd3-4645-b905-3cc298a94386","systemType":"Linux","status":"Created","lastRun":null,"filter":{"exclusionMask":[],"ignoreInaccessibleMachine":false,"osTypes":null,"applications":null,"customApplication":null,"platforms":null},"notificationSettings":{"isEnabled":false,"scheduleType":"Days","scheduleTime":"00:00","weekSettings":null,"to":null,"subject":null,"notifyOnTheFirstRun":false},"scheduleSettings":{"scheduleType":"Monthly","timeZone":{"timeZoneId":"Pacific Standard Time","displayName":"(UTC-08:00) Pacific Time (US & Canada)","utcOffset":180},"dailyScheduleSettings":null,"monthlyScheduleSettings":{"time":"11:00","calendarSettings":{"monthlyMode":"Day","weekDayNumber":null,"dayOfWeek":null,"day":2,"months":["Jun","Jul","Dec"]}},"periodicalScheduleSettings":null},"totalComputersCount":1,"onlineComputersCount":1,"offlineComputersCount":0}}},{"instanceUid":"b44ae0ed-5700-4637-bd4f-06193f68f002","method":"NetworkBased","credentials":[{"instanceUid":"00000004-0000-0000-0000-000000000000","username":"root","password":null,"priority":0,"description":"1","sshPort":22,"elevateAccountPrivileges":false,"addAccountToSudoersFile":false,"useSuIfsudoFails":false,"rootPassword":null,"sshPrivateKey":"••••••","passphrase":null,"type":"LinuxCertificate"}],"deploymentSettings":{"isEnabled":false,"backupPolicyUid":null,"setReadOnlyAccess":true},"_embedded":{"discoveryRule":{"instanceUid":"b44ae0ed-5700-4637-bd4f-06193f68f002","name":"Key Linux Network Rule","masterAgentUid":"da463bea-d186-4a14-b809-4aba366b03e8","locationUid":"757dd2a9-b65c-483a-823a-fbfa77cb911f","companyUid":"61204df8-9cd3-4645-b905-3cc298a94386","systemType":"Linux","status":"Success","lastRun":"2024-07-17T22:00:04.6937361-06:00","filter":{"exclusionMask":[],"ignoreInaccessibleMachine":true,"osTypes":["Debian","Ubuntu"],"applications":null,"customApplication":null,"platforms":["MicrosoftHyperVandVmWareVSphere","Physical"]},"notificationSettings":{"isEnabled":true,"scheduleType":"Weeks","scheduleTime":"05:00","weekSettings":{"scheduleDay":"Wednesday"},"to":"ivza@q.we","subject":"notify me","notifyOnTheFirstRun":false},"scheduleSettings":{"scheduleType":"NotScheduled","timeZone":{"timeZoneId":"Pacific Standard Time","displayName":"(UTC-08:00) Pacific Time (US & Canada)","utcOffset":-420},"dailyScheduleSettings":null,"monthlyScheduleSettings":null,"periodicalScheduleSettings":null},"totalComputersCount":2,"onlineComputersCount":2,"offlineComputersCount":0}}},{"instanceUid":"740f6271-5812-4af1-a20e-277c5beecc8d","method":"NetworkBased","credentials":[{"instanceUid":"00000006-0000-0000-0000-000000000000","username":"root","password":"••••••","priority":0,"description":"1","sshPort":22,"elevateAccountPrivileges":true,"addAccountToSudoersFile":true,"useSuIfsudoFails":true,"rootPassword":"••••••","sshPrivateKey":"••••••","passphrase":null,"type":"LinuxCertificate"}],"deploymentSettings":{"isEnabled":true,"backupPolicyUid":null,"setReadOnlyAccess":false},"_embedded":{"discoveryRule":{"instanceUid":"740f6271-5812-4af1-a20e-277c5beecc8d","name":"Linux Network Rule 2","masterAgentUid":"da463bea-d186-4a14-b809-4aba366b03e8","locationUid":"757dd2a9-b65c-483a-823a-fbfa77cb911f","companyUid":"61204df8-9cd3-4645-b905-3cc298a94386","systemType":"Linux","status":"Created","lastRun":null,"filter":{"exclusionMask":[],"ignoreInaccessibleMachine":true,"osTypes":["Debian","Ubuntu"],"applications":null,"customApplication":null,"platforms":["MicrosoftHyperVandVmWareVSphere","Physical"]},"notificationSettings":{"isEnabled":false,"scheduleType":"Weeks","scheduleTime":"05:00","weekSettings":{"scheduleDay":"Monday"},"to":"admin@alpha.com","subject":"notify me","notifyOnTheFirstRun":false},"scheduleSettings":{"scheduleType":"NotScheduled","timeZone":{"timeZoneId":"Pacific Standard Time","displayName":"(UTC-08:00) Pacific Time (US & Canada)","utcOffset":-420},"dailyScheduleSettings":null,"monthlyScheduleSettings":null,"periodicalScheduleSettings":null},"totalComputersCount":1,"onlineComputersCount":1,"offlineComputersCount":0}}},{"instanceUid":"f723d728-229b-4128-8160-2053673c4bdb","method":"Manual","credentials":[{"instanceUid":"00000003-0000-0000-0000-000000000000","username":"root","password":"••••••","priority":0,"description":"","sshPort":22,"elevateAccountPrivileges":false,"addAccountToSudoersFile":false,"useSuIfsudoFails":false,"rootPassword":"••••••","sshPrivateKey":null,"passphrase":null,"type":"LinuxBased"}],"deploymentSettings":{"isEnabled":false,"backupPolicyUid":null,"setReadOnlyAccess":true},"_embedded":{"discoveryRule":{"instanceUid":"f723d728-229b-4128-8160-2053673c4bdb","name":"Linux Custom Rule 1","masterAgentUid":"da463bea-d186-4a14-b809-4aba366b03e8","locationUid":"757dd2a9-b65c-483a-823a-fbfa77cb911f","companyUid":"61204df8-9cd3-4645-b905-3cc298a94386","systemType":"Linux","status":"Success","lastRun":"2024-07-17T21:47:34.5990319-06:00","filter":{"exclusionMask":[],"ignoreInaccessibleMachine":false,"osTypes":null,"applications":null,"customApplication":null,"platforms":null},"notificationSettings":{"isEnabled":false,"scheduleType":"Days","scheduleTime":"00:00","weekSettings":null,"to":null,"subject":null,"notifyOnTheFirstRun":false},"scheduleSettings":{"scheduleType":"NotScheduled","timeZone":{"timeZoneId":"Pacific Standard Time","displayName":"(UTC-08:00) Pacific Time (US & Canada)","utcOffset":-420},"dailyScheduleSettings":null,"monthlyScheduleSettings":null,"periodicalScheduleSettings":null},"totalComputersCount":2,"onlineComputersCount":2,"offlineComputersCount":0}}}],"meta":{"pagingInfo":{"total":4,"count":4,"offset":0}}}`

const guestOSCredentialsExample = `{"data":[{"guestOsCredentialsUid":"7466a1a1-1881-42b6-8f42-3758ccca133b","role":"serviceProviderAdministrator","username":"administrator","description":"","password":"������","organizationUid":"507e9b59-a73d-489d-ab0c-e34d23ded114","siteUid":"b2e721fc-82ef-4ca6-a1b2-5452bc4399b6","appliances":null,"lastChangeTimestamp":"2023-03-18T08:51:12.44-06:00"},{"guestOsCredentialsUid":"ed38845b-0e3d-45e3-aca6-1a589482b5ce","role":"serviceProviderAdministrator","username":"administratort2","description":"","password":"������","organizationUid":"507e9b59-a73d-489d-ab0c-e34d23ded114","siteUid":"b2e721fc-82ef-4ca6-a1b2-5452bc4399b6","appliances":null,"lastChangeTimestamp":"2023-11-07T10:35:09.69-07:00"}],"meta":{"pagingInfo":{"total":2,"count":2,"offset":0}}}`

// TestMaskedPasswordPlaceholderIsNotHeld (class masked-placeholder): the
// documented mask-bearing responses leave a plain default sync whole (exit 0,
// nothing withheld, fleet-health reads the agent jobs) and print on a direct
// read, CLI and MCP; each other mask VSPC writes does the same on the two
// partitions the finding named.
func TestMaskedPasswordPlaceholderIsNotHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "vspc-" + "M4skT0kQ8zLw"
	documented := map[string]string{
		"/configuration/backupPolicies/linux":                  linuxPoliciesExample,
		"/configuration/backupPolicies/windows":                windowsPoliciesExample,
		"/discovery/rules/linux":                               linuxDiscoveryRulesExample,
		"/infrastructure/sites/publicCloud/guestOsCredentials": guestOSCredentialsExample,
		"/infrastructure/backupAgents/jobs":                    agentJobsExample,
	}
	serve := func(t *testing.T, mask string) []string {
		v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
			for path, body := range documented {
				if strings.HasSuffix(r.URL.Path, path) {
					io.WriteString(w, strings.ReplaceAll(body, "••••••", mask))
					return
				}
			}
			io.WriteString(w, emptyPage)
		})
		return closedEnv(t.TempDir(), v.env(token)...)
	}
	printed := func(t *testing.T, label, out, want string) {
		t.Helper()
		if !strings.Contains(out, want) || strings.Contains(out, "withheld") {
			t.Errorf("%s did not print %q:\n%s", label, want, out)
		}
	}
	syncDone := func(t *testing.T, out string, code int, resources ...string) {
		t.Helper()
		if code != 0 {
			t.Errorf("sync exited %d:\n%s", code, out)
		}
		for _, res := range resources {
			if !strings.Contains(out, `"event":"sync_complete","resource":"`+res+`"`) {
				t.Errorf("sync did not complete %s:\n%s", res, out)
			}
		}
		if strings.Contains(out, "withheld") {
			t.Errorf("sync withheld a unit that carried only a mask:\n%s", out)
		}
	}

	t.Run("default sync", func(t *testing.T) {
		env := serve(t, "••••••")
		out, code := runCode(t, cliBin, env, "sync")
		syncDone(t, out, code, "configuration-backup-policies-linux", "configuration-backup-policies-windows",
			"discovery-rules-linux", "infrastructure-sites-public-cloud-guest-os-credentials", "infrastructure")
		fh, code := runCode(t, cliBin, env, "fleet-health", "--json")
		if code != 0 || !strings.Contains(fh, `"jobs_total": 4`) {
			t.Errorf("fleet-health after a default sync exited %d, want the 4 agent jobs:\n%s", code, fh)
		}
		printed(t, "search (local store)", run(t, cliBin, env, "search", "LServerVolumeCloud", "--json"), "LServerVolumeCloud")
		printed(t, "configuration get-linux-backup-policies", run(t, cliBin, env, "configuration", "get-linux-backup-policies", "--json", "--data-source", "live"), "LServerVolumeCloud")
		printed(t, "configuration get-windows-backup-policies", run(t, cliBin, env, "configuration", "get-windows-backup-policies", "--json", "--data-source", "live"), "••••••")
		printed(t, "discovery get-linux-rules", run(t, cliBin, env, "discovery", "get-linux-rules", "--json", "--data-source", "live"), `"sshPrivateKey": "••••••"`)
		printed(t, "infrastructure get-public-cloud-guest-os-credentials", run(t, cliBin, env, "infrastructure", "get-public-cloud-guest-os-credentials", "--json", "--data-source", "live"), "guestOsCredentialsUid")
		printed(t, "MCP veeam_read configuration.get-linux-backup-policies",
			mcpCall(t, mcpBin, env, `{"name":"veeam_read","arguments":{"endpoint_id":"configuration.get-linux-backup-policies"}}`), "LServerVolumeCloud")
	})

	for _, mask := range []string{"******", "\uFFFD\uFFFD\uFFFD\uFFFD\uFFFD\uFFFD", "●●●●●●●●●●●●"} {
		t.Run(mask, func(t *testing.T) {
			env := serve(t, mask)
			out, code := runCode(t, cliBin, env, "sync", "--concurrency", "1", "--resources", "configuration-backup-policies-linux,infrastructure")
			syncDone(t, out, code, "configuration-backup-policies-linux", "infrastructure")
			printed(t, "search (local store)", run(t, cliBin, env, "search", "LServerVolumeCloud", "--json"), "LServerVolumeCloud")
			printed(t, "configuration get-linux-backup-policies", run(t, cliBin, env, "configuration", "get-linux-backup-policies", "--json", "--data-source", "live"), "LServerVolumeCloud")
		})
	}
}

// TestRealValueUnderMaskedFieldNamesHeld (class masked-placeholder, the other
// direction): password, rootPassword and sshPrivateKey are held in connector
// code, not by the kit's response declaration, so every value that is not a
// mask must still be held before any sink: a real password, one in a
// duplicate key, one under an escaped or upper-case key, a number, values
// nested beneath the name, one in a body that is not JSON, one in a refused
// redirect's Location and one in a response header.
func TestRealValueUnderMaskedFieldNamesHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "vspc-" + "R3alV4lT0k9q"
	const agentPassword = "ma7Q/vLx" + "+2Rk9Wp4"
	const dupPassword = "dp" + "K8wN3qZ6vR1x"
	const escPassword = "ep" + "T5yH2mL9cW4b"
	const rootPassword = "rp" + "J3vB8nQ1zX6k"
	const sshKey = "sk" + "P9rL4tW7mF2d"
	const numPassword = "73915864" + "20"
	const textPassword = "tp" + "Z6cV1bN8xM3q"
	const redirectPassword = "lp" + "W2xK7cR4vN9m"
	const headerPassword = "hp" + "B5nM8qL3zT6w"
	const pairPassword = "cp" + "F4jD9sG2kH7v"
	secrets := []string{agentPassword, dupPassword, escPassword, rootPassword, sshKey, numPassword, textPassword,
		redirectPassword, headerPassword, pairPassword}
	elsewhere := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"data":{}}`) })
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/managementAgents/ma4/credentials"):
			w.Header().Set("Location", elsewhere.url+"/landing?password="+redirectPassword)
			w.WriteHeader(http.StatusFound)
		case strings.HasSuffix(r.URL.Path, "/managementAgents/ma5/credentials"):
			w.Header().Set("Password", headerPassword)
			w.Header().Set("cf-mitigated", "password="+pairPassword)
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"errors":[{"message":"blocked"}]}`)
		case strings.HasSuffix(r.URL.Path, "/managementAgents/ma1/credentials"):
			io.WriteString(w, `{"data":{"username":"VSPC\\admin","password":"`+agentPassword+`"}}`)
		case strings.HasSuffix(r.URL.Path, "/managementAgents/ma2/credentials"):
			io.WriteString(w, `{"data":{"username":"u","password":"••••••","password":"`+dupPassword+`",`+
				`"p\u0061ssword":"`+escPassword+`","details":{"ROOTPASSWORD":["`+rootPassword+`"],"keys":[{"sshPrivateKey":{"pem":"`+sshKey+`"}}]},`+
				`"Password":`+numPassword+`}}`)
		case strings.HasSuffix(r.URL.Path, "/managementAgents/ma3/credentials"):
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "username=u\npassword = "+textPassword+"\n")
		default:
			io.WriteString(w, emptyPage)
		}
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	var outs []string
	for _, uid := range []string{"ma1", "ma2", "ma3", "ma4", "ma5"} {
		outs = append(outs,
			run(t, cliBin, env, "infrastructure", "get-management-agent-credentials", uid, "--json"),
			run(t, cliBin, env, "infrastructure", "get-management-agent-credentials", uid, "--agent"),
			mcpCall(t, mcpBin, env, `{"name":"veeam_read","arguments":{"endpoint_id":"infrastructure.get-management-agent-credentials","params":{"managementAgentUid":"`+uid+`"}}}`))
	}
	if v.hits.Load() < 15 {
		t.Errorf("only %d request(s) reached the API stub", v.hits.Load())
	}
	for _, s := range secrets {
		for _, sp := range spellings(s) {
			for _, out := range outs {
				if strings.Contains(out, sp) {
					t.Errorf("output shows %q (a spelling of %q):\n%s", sp, s, out)
				}
			}
			assertNoFileSpells(t, home, s, sp)
		}
	}
}

// spellings is s as written and as the encodings a sink could carry it in:
// base64 (both alphabets, every alignment), hex (both cases) and URL-encoded.
func spellings(s string) []string {
	out := []string{s, hex.EncodeToString([]byte(s)), strings.ToUpper(hex.EncodeToString([]byte(s))), url.QueryEscape(s), url.PathEscape(s)}
	for shift := 0; shift < 3; shift++ {
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
			b := enc.EncodeToString([]byte(strings.Repeat("\x00", shift) + s))
			lo := 0
			if shift > 0 {
				lo = 4 // the quantum that carries the shift
			}
			out = append(out, b[lo:len(b)-4])
		}
	}
	return out
}

// assertNoFileSpells walks home for the spelling sp of secret s: the response
// cache, the SQLite store, the journal and every other file the run wrote.
func assertNoFileSpells(t *testing.T, home, s, sp string) {
	t.Helper()
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr == nil && strings.Contains(string(data), sp) {
			t.Errorf("%s holds %q (a spelling of %q)", p, sp, s)
		}
		return nil
	})
}
