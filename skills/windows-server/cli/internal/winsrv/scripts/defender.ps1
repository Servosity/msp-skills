if ($null -eq (Get-Service -Name WinDefend -ErrorAction SilentlyContinue)) { Emit-Err 'role_absent' 'Microsoft Defender Antivirus is not installed' }
$s = Get-MpComputerStatus
Emit-Ok ([ordered]@{
  am_service_enabled = [bool]$s.AMServiceEnabled
  antivirus_enabled = [bool]$s.AntivirusEnabled
  real_time_protection_enabled = [bool]$s.RealTimeProtectionEnabled
  signature_last_updated = (Iso $s.AntivirusSignatureLastUpdated)
  signature_age_days = [int64]$s.AntivirusSignatureAge
  quick_scan_age_days = [int64]$s.QuickScanAge
  full_scan_age_days = [int64]$s.FullScanAge
  running_mode = (Str $s.AMRunningMode)
  tamper_protected = [bool]$s.IsTamperProtected
})
