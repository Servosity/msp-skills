$log = Need-Enum 'log' ([string](Arg 'log' 'System')) @('System', 'Application')
$hours = Need-Int 'hours' (Arg 'hours' 24) 1 720
$max = Need-Int 'max' (Arg 'max' 200) 1 2000
$levels = @(Arg 'levels' @(1, 2, 3)) | ForEach-Object { Need-Int 'levels' $_ 1 4 }
$filter = @{ LogName = $log; Level = [int[]]$levels; StartTime = (Get-Date).AddHours(-$hours) }
$events = @()
try { $events = @(Get-WinEvent -FilterHashtable $filter -MaxEvents $max -ErrorAction Stop) }
catch { if ([string]$_.FullyQualifiedErrorId -like 'NoMatchingEventsFound*') { Emit-Ok @() } else { throw } }
$rows = New-Object System.Collections.ArrayList
foreach ($e in $events) {
  $msg = [string]$e.Message
  if ($msg.Length -gt 1000) { $msg = $msg.Substring(0, 1000) }
  [void]$rows.Add([ordered]@{
    log = $log
    record_id = [int64]$e.RecordId
    time_created = (Iso $e.TimeCreated)
    level = [int]$e.Level
    level_name = (Str $e.LevelDisplayName)
    provider = (Str $e.ProviderName)
    event_id = [int]$e.Id
    message = $msg
  })
}
Emit-Ok $rows.ToArray()
