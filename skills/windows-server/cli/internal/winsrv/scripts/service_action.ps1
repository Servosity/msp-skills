# The only script that changes anything. The CLI sends it only after preview
# and an explicit confirmation; it re-checks every refusal before acting.
$n = Need-Name 'name' (Arg 'name' '')
$action = Need-Enum 'action' ([string](Arg 'action' '')) @('restart', 'start')
$plan = Service-Plan $n $action
if (-not $plan.allowed) { Emit-Err 'refused' $plan.refusal }
# Service-Plan allows start only for a stopped service, so start always changes it.
if ($action -eq 'start') { Start-Service -Name $n }
else { Restart-Service -Name $n }
$svc = Get-Service -Name $n
try { $svc.WaitForStatus('Running', [TimeSpan]::FromSeconds(30)) } catch { }
$svc.Refresh()
$plan['state_after'] = [string]$svc.Status
$plan['changed'] = $true
Emit-Ok $plan
