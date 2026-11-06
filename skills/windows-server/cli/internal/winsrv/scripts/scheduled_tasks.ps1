$includeMs = [bool](Arg 'include_microsoft' $false)
$rows = New-Object System.Collections.ArrayList
foreach ($t in @(Get-ScheduledTask)) {
  if (-not $includeMs -and [string]$t.TaskPath -like '\Microsoft\*') { continue }
  $i = $null; $infoErr = $null
  try { $i = $t | Get-ScheduledTaskInfo -ErrorAction Stop } catch { $infoErr = [string]$_.Exception.Message }
  $last = $null; $next = $null; $result = $null
  if ($null -ne $i) { $last = Iso $i.LastRunTime; $next = Iso $i.NextRunTime; $result = [int64]$i.LastTaskResult }
  [void]$rows.Add([ordered]@{
    task_path = (Str $t.TaskPath)
    task_name = (Str $t.TaskName)
    state = (Str $t.State)
    run_as = (Str $t.Principal.UserId)
    last_run_time = $last
    last_result = $result
    next_run_time = $next
    info_error = $infoErr
  })
}
Emit-Ok $rows.ToArray()
