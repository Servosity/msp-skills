$days = Need-Int 'days' (Arg 'days' 7) 1 30
$max = Need-Int 'max' (Arg 'max' 500) 1 2000
$filter = @{ LogName = 'System'; ProviderName = 'Service Control Manager'; Id = 7000, 7009, 7011, 7023, 7024, 7031, 7034; StartTime = (Get-Date).AddDays(-$days) }
$events = @()
try { $events = @(Get-WinEvent -FilterHashtable $filter -MaxEvents $max -ErrorAction Stop) }
catch { if ([string]$_.FullyQualifiedErrorId -like 'NoMatchingEventsFound*') { Emit-Ok @() } else { throw } }
$rows = New-Object System.Collections.ArrayList
foreach ($e in $events) {
  $params = New-Object System.Collections.ArrayList
  $keyName = $null
  foreach ($p in @($e.Properties)) {
    if ($p.Value -is [byte[]]) { $keyName = ([System.Text.Encoding]::Unicode.GetString($p.Value)).TrimEnd([char]0) }
    else { [void]$params.Add([string]$p.Value) }
  }
  $msg = [string]$e.Message
  if ($msg.Length -gt 500) { $msg = $msg.Substring(0, 500) }
  [void]$rows.Add([ordered]@{
    event_id = [int]$e.Id
    record_id = [int64]$e.RecordId
    time_created = (Iso $e.TimeCreated)
    service_key = $keyName
    params = $params.ToArray()
    message = $msg
  })
}
Emit-Ok $rows.ToArray()
