# Errors and warnings grouped by (log, provider, event id) over the window
# plus the baseline before it, counted on the server so only groups travel.
$window = Need-Int 'window_hours' (Arg 'window_hours' 24) 1 168
$baseline = Need-Int 'baseline_hours' (Arg 'baseline_hours' 168) 1 720
$cap = 20000
$now = Get-Date
$windowStart = $now.AddHours(-$window)
$start = $windowStart.AddHours(-$baseline)
$groups = @{}
$oldest = @{}
$total = 0
$capped = $false
foreach ($log in @('System', 'Application')) {
  try { $o = @(Get-WinEvent -LogName $log -MaxEvents 1 -Oldest -ErrorAction Stop); if ($o.Count -gt 0) { $oldest[$log] = (Iso $o[0].TimeCreated) } } catch { if ([string]$_.FullyQualifiedErrorId -notlike 'NoMatchingEventsFound*') { throw }; $oldest[$log] = $null }
  $events = @()
  try { $events = @(Get-WinEvent -FilterHashtable @{ LogName = $log; Level = 1, 2, 3; StartTime = $start } -MaxEvents $cap -ErrorAction Stop) }
  catch { if ([string]$_.FullyQualifiedErrorId -notlike 'NoMatchingEventsFound*') { throw } }
  $total += $events.Count
  if ($events.Count -ge $cap) { $capped = $true }
  foreach ($e in $events) {
    $k = $log + '|' + [string]$e.ProviderName + '|' + [string]$e.Id
    $g = $groups[$k]
    if ($null -eq $g) {
      $g = @{ log = $log; provider = [string]$e.ProviderName; event_id = [int]$e.Id; level = [int]$e.Level; window = 0; baseline = 0; first = $null; last = $null; sample = $null }
      $groups[$k] = $g
    }
    if ($e.TimeCreated -ge $windowStart) {
      $g.window++
      if ($null -eq $g.last) { $g.last = $e.TimeCreated; $g.sample = $e }
      $g.first = $e.TimeCreated
    } else { $g.baseline++ }
  }
}
$rows = New-Object System.Collections.ArrayList
foreach ($g in $groups.Values) {
  $msg = $null
  if ($null -ne $g.sample) { $msg = ([string]$g.sample.Message -replace '\s+', ' ').Trim(); if ($msg.Length -gt 300) { $msg = $msg.Substring(0, 297) + '...' } }
  [void]$rows.Add([ordered]@{ log = $g.log; provider = $g.provider; event_id = $g.event_id; level = $g.level; window_count = $g.window; baseline_count = $g.baseline; first_seen = (Iso $g.first); last_seen = (Iso $g.last); sample_message = $msg })
}
Emit-Ok ([ordered]@{ window_hours = $window; baseline_hours = $baseline; window_start = (Iso $windowStart); baseline_start = (Iso $start); oldest_record = $oldest; events_read = $total; capped = $capped; groups = $rows.ToArray() })
