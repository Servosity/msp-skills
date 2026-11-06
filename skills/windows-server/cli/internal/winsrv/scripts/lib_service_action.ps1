# Service-Plan decides whether a restart or start may run, without changing anything.
# Refused: WinRM itself and every service WinRM depends on (restarting one
# drops this session), a restart with running dependents (no -Force is ever
# used), a restart of a stopped service, and a start of anything but a
# stopped automatic service.
function Service-Plan([string]$n, [string]$action) {
  $s = @(Get-Service -Name $n -ErrorAction SilentlyContinue | Where-Object { [string]$_.Name -eq $n })
  if ($s.Count -eq 0) { Emit-Err 'not_found' 'No service with that name' }
  $s = $s[0]
  $cim = @(Get-CimInstance -ClassName Win32_Service | Where-Object { [string]$_.Name -eq $n })[0]
  $protect = New-Object System.Collections.ArrayList
  $queue = New-Object System.Collections.Queue
  $queue.Enqueue((Get-Service -Name WinRM))
  while ($queue.Count -gt 0) {
    $q = $queue.Dequeue()
    if ($protect -contains [string]$q.Name) { continue }
    [void]$protect.Add([string]$q.Name)
    foreach ($d in @($q.ServicesDependedOn)) { $queue.Enqueue($d) }
  }
  $running = @($s.DependentServices | Where-Object { [string]$_.Status -eq 'Running' } | ForEach-Object { [string]$_.Name })
  $refusal = $null
  if ($protect -contains [string]$s.Name) { $refusal = 'WinRM needs this service; restarting or starting it here would drop the session' }
  elseif ($action -eq 'restart' -and [string]$s.Status -ne 'Running') { $refusal = 'the service is not running; use services start' }
  elseif ($action -eq 'restart' -and $running.Count -gt 0) { $refusal = 'running services depend on it: ' + ($running -join ', ') }
  elseif ($action -eq 'start' -and [string]$s.Status -ne 'Stopped') { $refusal = 'the service is not stopped' }
  elseif ($action -eq 'start' -and [string]$cim.StartMode -ne 'Auto') { $refusal = 'only an automatic service is started here; its start mode is ' + [string]$cim.StartMode }
  return [ordered]@{
    name = [string]$s.Name
    display_name = [string]$s.DisplayName
    action = $action
    state_before = [string]$s.Status
    start_mode = [string]$cim.StartMode
    running_dependents = $running
    allowed = ($null -eq $refusal)
    refusal = $refusal
  }
}
