// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"strings"
)

// windowsEnv is a child's envelope on a Windows CI runner (GitHub's
// windows-latest image, about 9 KB): the program, its command line, and an
// environment of Windows paths. Backslashes run through every value, so
// string escapes (\a, \b, \t, \n, \v, \x64, octal \17) occur in ordinary
// paths. kit11rv: the kit exhausted its budget on this shape and refused
// to start any child there.
func windowsEnv() []byte {
	hc := `C:\hostedtoolcache\windows`
	jdk := func(v string) string { return hc + `\Java_Temurin-Hotspot_jdk\` + v + `\x64` }
	path := strings.Join([]string{
		`C:\Program Files\PowerShell\7`, hc + `\go\1.26.0\x64\bin`, `C:\Users\runneradmin\go\bin`,
		`C:\Program Files\MongoDB\Server\7.0\bin`, `C:\vcpkg`, `C:\tools\zstd`, `C:\Program Files\Mercurial\`,
		hc + `\stack\3.7.1\x64`, `C:\cabal\bin`, `C:\\ghcup\bin`, `C:\mingw64\bin`, `C:\Program Files\dotnet`,
		`C:\Program Files\MySQL\MySQL Server 8.0\bin`, `C:\Program Files\R\R-4.5.1\bin\x64`,
		`C:\SeleniumWebDrivers\GeckoDriver`, `C:\SeleniumWebDrivers\EdgeDriver\`, `C:\SeleniumWebDrivers\ChromeDriver`,
		`C:\Program Files (x86)\sbt\bin`, `C:\Program Files (x86)\GitHub CLI`, `C:\Program Files\Git\bin`,
		`C:\Program Files (x86)\pipx_bin`, `C:\npm\prefix`, hc + `\Python\3.12.10\x64\Scripts`, hc + `\Python\3.12.10\x64`,
		hc + `\Ruby\3.3.9\x64\bin`, `C:\Program Files\OpenSSL\bin`, `C:\tools\kotlinc\bin`, jdk(`17.0.16-8`) + `\bin`,
		`C:\Program Files (x86)\Microsoft SDKs\Azure\CLI2\wbin`, `C:\ProgramData\kind`, `C:\Windows\system32`, `C:\Windows`,
		`C:\Windows\System32\Wbem`, `C:\Windows\System32\WindowsPowerShell\v1.0\`, `C:\Windows\System32\OpenSSH\`,
		`C:\Program Files\dotnet\`, `C:\ProgramData\Chocolatey\bin`, `C:\Program Files\PowerShell\7\`,
		`C:\Program Files\Microsoft\Web Platform Installer\`, `C:\Program Files\Microsoft SQL Server\Client SDK\ODBC\170\Tools\Binn\`,
		`C:\Program Files\Microsoft SQL Server\150\Tools\Binn\`, `C:\Program Files\nodejs\`, `C:\Program Files\Git\cmd`,
		`C:\Program Files\Git\mingw64\bin`, `C:\Program Files\Git\usr\bin`, `C:\Program Files\GitHub CLI\`, `c:\tools\php`,
		`C:\Program Files\Amazon\AWSCLIV2\`, `C:\Program Files\Amazon\SessionManagerPlugin\bin\`,
		`C:\Program Files\Amazon\AWSSAMCLI\bin\`, `C:\Program Files\Microsoft SQL Server\130\Tools\Binn\`,
		`C:\Program Files\LLVM\bin`, `C:\Users\runneradmin\.dotnet\tools`, `C:\Users\runneradmin\.cargo\bin`,
		`C:\Users\runneradmin\AppData\Local\Microsoft\WindowsApps`, `C:\Program Files\Docker`, `C:\Program Files\CMake\bin`,
		`C:\Program Files\Microsoft Service Fabric\bin\Fabric\Fabric.Code`, `C:\Program Files\Microsoft SDKs\Service Fabric\Tools\ServiceFabricLocalClusterManager`,
		`C:\Program Files\PostgreSQL\17\bin`, `C:\Program Files\Mozilla Firefox`, `C:\Strawberry\c\bin`, `C:\Strawberry\perl\site\bin`, `C:\Strawberry\perl\bin`,
	}, ";")
	lines := []string{
		hc + `\go\1.26.0\x64\pkg\tool\windows_amd64\..\..\..\..\go-build3381272611\b001\credkit.test.exe`,
		`D:\a\_temp\go-build3381272611\b001\credkit.test.exe -test.run=^$`,
		"ACTIONS_RUNNER_RETURN_JOB_RESULT_FOR_HOSTED=1", "AGENT_TOOLSDIRECTORY=" + hc, `ALLUSERSPROFILE=C:\ProgramData`,
		`ANDROID_HOME=C:\Android\android-sdk`, `ANDROID_NDK=C:\Android\android-sdk\ndk\27.3.13750724`,
		`ANDROID_NDK_HOME=C:\Android\android-sdk\ndk\27.3.13750724`, `ANDROID_NDK_LATEST_HOME=C:\Android\android-sdk\ndk\29.0.14206865`,
		`ANDROID_NDK_ROOT=C:\Android\android-sdk\ndk\27.3.13750724`, `ANDROID_SDK_ROOT=C:\Android\android-sdk`,
		`ANT_HOME=C:\ProgramData\chocolatey\lib\ant\tools\apache-ant-1.10.15`, `AZURE_CONFIG_DIR=C:\azureCli`,
		`AZURE_EXTENSION_DIR=C:\Program Files\Common Files\AzureCliExtensionDirectory`, `AZ_DEVOPS_GLOBAL_CONFIG_DIR=C:\azureDevOpsCli`,
		`CABAL_DIR=C:\cabal`, `ChocolateyInstall=C:\ProgramData\chocolatey`, `ChromeWebDriver=C:\SeleniumWebDrivers\ChromeDriver`,
		"CI=true", `COBERTURA_HOME=C:\cobertura-2.1.1`, `CommonProgramFiles=C:\Program Files\Common Files`,
		`CommonProgramFiles(x86)=C:\Program Files (x86)\Common Files`, `CommonProgramW6432=C:\Program Files\Common Files`,
		`COMPUTERNAME=RUNNERVM`, `ComSpec=C:\Windows\system32\cmd.exe`, `CONDA=C:\Miniconda`, `DOTNET_MULTILEVEL_LOOKUP=0`,
		`DOTNET_NOLOGO=1`, `DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1`, `DriverData=C:\Windows\System32\Drivers\DriverData`,
		`EdgeWebDriver=C:\SeleniumWebDrivers\EdgeDriver`, `GeckoWebDriver=C:\SeleniumWebDrivers\GeckoDriver`,
		`GHCUP_INSTALL_BASE_PREFIX=C:\`, `GHCUP_MSYS2=C:\msys64`, `GITHUB_ACTION=__run_2`, `GITHUB_ACTIONS=true`,
		`GITHUB_ACTOR=dstevens-bot`, `GITHUB_ACTOR_ID=81234567`, `GITHUB_API_URL=https://api.github.com`,
		`GITHUB_ARTIFACTS_LIST=D:\a\_temp\_runner_file_commands\artifacts_list_5e0f8a2c-6b1d-4c3e-9f7a-2d8b1e4c6a90`,
		`GITHUB_BASE_REF=main`, `GITHUB_ENV=D:\a\_temp\_runner_file_commands\set_env_8b2e1d4c-3f6a-4e9b-a1c7-5d0f2e8b9c31`,
		`GITHUB_EVENT_NAME=pull_request`, `GITHUB_EVENT_PATH=D:\a\_temp\_github_workflow\event.json`,
		`GITHUB_GRAPHQL_URL=https://api.github.com/graphql`, `GITHUB_HEAD_REF=tools/credkit`, `GITHUB_JOB=credkit-windows`,
		`GITHUB_OUTPUT=D:\a\_temp\_runner_file_commands\set_output_0c9d8e7f-6a5b-4c3d-2e1f-0a9b8c7d6e5f`,
		`GITHUB_PATH=D:\a\_temp\_runner_file_commands\add_path_1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d`,
		`GITHUB_REF=refs/pull/500/merge`, `GITHUB_REF_NAME=500/merge`, `GITHUB_REF_PROTECTED=false`, `GITHUB_REF_TYPE=branch`,
		`GITHUB_REPOSITORY=Servosity/msp-skills`, `GITHUB_REPOSITORY_ID=912345678`, `GITHUB_REPOSITORY_OWNER=Servosity`,
		`GITHUB_REPOSITORY_OWNER_ID=123456789`, `GITHUB_RETENTION_DAYS=90`, `GITHUB_RUN_ATTEMPT=1`, `GITHUB_RUN_ID=41525397190`,
		`GITHUB_RUN_NUMBER=8812`, `GITHUB_SERVER_URL=https://github.com`,
		`GITHUB_SHA=dcb608d4a0e1f2c3b4a5968778695a4b3c2d1e0f`,
		`GITHUB_STATE=D:\a\_temp\_runner_file_commands\save_state_7f6e5d4c-3b2a-1908-f7e6-d5c4b3a29180`,
		`GITHUB_STEP_SUMMARY=D:\a\_temp\_runner_file_commands\step_summary_2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e`,
		`GITHUB_TRIGGERING_ACTOR=dstevens-bot`, `GITHUB_WORKFLOW=ci`,
		`GITHUB_WORKFLOW_REF=Servosity/msp-skills/.github/workflows/ci.yml@refs/pull/500/merge`,
		`GITHUB_WORKFLOW_SHA=dcb608d4a0e1f2c3b4a5968778695a4b3c2d1e0f`, `GITHUB_WORKSPACE=D:\a\msp-skills\msp-skills`,
		`GOROOT=` + hc + `\go\1.26.0\x64`, `GOROOT_1_24_X64=` + hc + `\go\1.24.11\x64`, `GOROOT_1_25_X64=` + hc + `\go\1.25.6\x64`,
		`GOROOT_1_26_X64=` + hc + `\go\1.26.0\x64`, `GRADLE_HOME=C:\ProgramData\chocolatey\lib\gradle\tools\gradle-9.1.0`,
		`HOMEDRIVE=C:`, `HOMEPATH=\Users\runneradmin`, `IEWebDriver=C:\SeleniumWebDrivers\IEDriver`, `ImageOS=win25`,
		`ImageVersion=20261026.112.1`, `JAVA_HOME=` + jdk(`17.0.16-8`), `JAVA_HOME_11_X64=` + jdk(`11.0.28-6`),
		`JAVA_HOME_17_X64=` + jdk(`17.0.16-8`), `JAVA_HOME_21_X64=` + jdk(`21.0.8-9`), `JAVA_HOME_25_X64=` + jdk(`25.0.0-36`),
		`JAVA_HOME_8_X64=` + jdk(`8.0.462-8`), `LOCALAPPDATA=C:\Users\runneradmin\AppData\Local`, `LOGONSERVER=\\RUNNERVM`,
		`M2=C:\ProgramData\chocolatey\lib\maven\apache-maven-3.9.11\bin`, `M2_REPO=C:\ProgramData\m2`, `MAVEN_OPTS=-Xms256m`,
		`npm_config_prefix=C:\npm\prefix`, `NUMBER_OF_PROCESSORS=4`, `OS=Windows_NT`, "PATH=" + path,
		`PATHEXT=.COM;.EXE;.BAT;.CMD;.VBS;.VBE;.JS;.JSE;.WSF;.WSH;.MSC;.CPL`, `PGBIN=C:\Program Files\PostgreSQL\17\bin`,
		`PGDATA=C:\Program Files\PostgreSQL\17\data`, `PGPASSWORD=root`, `PGROOT=C:\Program Files\PostgreSQL\17`,
		`PGUSER=postgres`, `PHPROOT=c:\tools\php`, `PIPX_BIN_DIR=C:\Program Files (x86)\pipx_bin`,
		`PIPX_HOME=C:\Program Files (x86)\pipx`, `POWERSHELL_DISTRIBUTION_CHANNEL=GitHub-Actions-win25`,
		`POWERSHELL_UPDATECHECK=Off`, `PROCESSOR_ARCHITECTURE=AMD64`,
		`PROCESSOR_IDENTIFIER=AMD64 Family 25 Model 1 Stepping 1, AuthenticAMD`, `PROCESSOR_LEVEL=25`,
		`PROCESSOR_REVISION=0101`, `ProgramData=C:\ProgramData`, `ProgramFiles=C:\Program Files`,
		`ProgramFiles(x86)=C:\Program Files (x86)`, `ProgramW6432=C:\Program Files`,
		`PSModuleAnalysisCachePath=C:\PSModuleAnalysisCachePath\ModuleAnalysisCache`,
		`PSModulePath=C:\Users\runneradmin\Documents\PowerShell\Modules;C:\Program Files\PowerShell\Modules;c:\program files\powershell\7\Modules;C:\\Modules\azurerm_2.1.0;C:\\Modules\azure_2.1.0;C:\Users\packer\Documents\WindowsPowerShell\Modules;C:\Program Files\WindowsPowerShell\Modules;C:\Windows\system32\WindowsPowerShell\v1.0\Modules;C:\Program Files\Microsoft SQL Server\130\Tools\PowerShell\Modules\`,
		`PUBLIC=C:\Users\Public`, `PWD=D:\a\msp-skills\msp-skills\tools\maintainer\credkit`, `RTOOLS45_HOME=C:\rtools45`,
		`RUNNER_ARCH=X64`, `RUNNER_ENVIRONMENT=github-hosted`, `RUNNER_NAME=GitHub Actions 1000123456`, `RUNNER_OS=Windows`,
		`RUNNER_TEMP=D:\a\_temp`, `RUNNER_TOOL_CACHE=` + hc, `RUNNER_TRACKING_ID=github_3d1f9a2b-8c7e-4f6a-b5d4-0e9c8b7a6f51`,
		`RUNNER_WORKSPACE=D:\a\msp-skills`, `SBT_HOME=C:\Program Files (x86)\sbt\`, `SELENIUM_JAR_PATH=C:\selenium\selenium-server.jar`,
		`SystemDrive=C:`, `SystemRoot=C:\Windows`, `TEMP=C:\Users\RUNNER~1\AppData\Local\Temp`,
		`TMP=C:\Users\RUNNER~1\AppData\Local\Temp`, `USERDOMAIN=RUNNERVM`, `USERDOMAIN_ROAMINGPROFILE=RUNNERVM`,
		`USERNAME=runneradmin`, `USERPROFILE=C:\Users\runneradmin`, `VCPKG_INSTALLATION_ROOT=C:\vcpkg`, `windir=C:\Windows`,
		`WIX=C:\Program Files (x86)\WiX Toolset v3.14\`, "CREDKIT_TEST_CHILD=held", "CREDKIT_HELD=handle:412",
	}
	return []byte(strings.Join(lines, "\n"))
}
