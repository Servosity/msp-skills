$os = Get-CimInstance -ClassName Win32_OperatingSystem
$cs = Get-CimInstance -ClassName Win32_ComputerSystem
$reasons = New-Object System.Collections.ArrayList
if (Test-Path -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') { [void]$reasons.Add('component_based_servicing') }
if (Test-Path -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired') { [void]$reasons.Add('windows_update') }
$sm = Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager' -ErrorAction SilentlyContinue
if ($null -ne $sm -and $null -ne $sm.PSObject.Properties['PendingFileRenameOperations'] -and $sm.PendingFileRenameOperations) { [void]$reasons.Add('pending_file_rename') }
$boot = $os.LastBootUpTime
Emit-Ok ([ordered]@{
  computer_name = (Str $cs.Name)
  domain = (Str $cs.Domain)
  os_caption = (Str $os.Caption)
  os_version = (Str $os.Version)
  os_build = (Str $os.BuildNumber)
  last_boot = (Iso $boot)
  uptime_seconds = [int64]((Get-Date) - $boot).TotalSeconds
  pending_reboot = ($reasons.Count -gt 0)
  pending_reboot_reasons = $reasons.ToArray()
  total_memory_bytes = [int64]$os.TotalVisibleMemorySize * 1024
  free_memory_bytes = [int64]$os.FreePhysicalMemory * 1024
})
