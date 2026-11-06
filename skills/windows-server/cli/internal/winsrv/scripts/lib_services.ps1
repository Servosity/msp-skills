function Service-Row($s) {
  $key = 'HKLM:\SYSTEM\CurrentControlSet\Services\' + [string]$s.Name
  $trigger = Test-Path -LiteralPath ($key + '\TriggerInfo')
  $delayed = $false
  if ($null -ne $s.PSObject.Properties['DelayedAutoStart']) { $delayed = [bool]$s.DelayedAutoStart }
  return [ordered]@{
    name = (Str $s.Name)
    display_name = (Str $s.DisplayName)
    state = (Str $s.State)
    start_mode = (Str $s.StartMode)
    delayed_auto_start = $delayed
    trigger_start = $trigger
    account = (Str $s.StartName)
    process_id = [int64]$s.ProcessId
    exit_code = [int64]$s.ExitCode
    service_exit_code = [int64]$s.ServiceSpecificExitCode
  }
}
