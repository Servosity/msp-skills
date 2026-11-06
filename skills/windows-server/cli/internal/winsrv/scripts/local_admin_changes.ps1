# Security events 4732 (member added) and 4733 (member removed) for the
# Administrators group, structured fields only (no rendered message text),
# plus the audit policy that decides whether such events are written at all.
$hours = Need-Int 'hours' (Arg 'hours' 168) 1 720
$audit = $null
try {
  # /r prints a header, a blank line, then the row; keep the non-blank lines.
  $csv = @(auditpol.exe /get '/subcategory:{0CCE9237-69AE-11D9-BED3-505054503030}' /r | Where-Object { ([string]$_).Trim() })
  if ($csv.Count -ge 2) { $audit = ([string]$csv[1] -split ',')[4] }
} catch { $audit = $null }
$changes = New-Object System.Collections.ArrayList
# The Administrators filter is part of the query, so -MaxEvents (which keeps
# the newest matches) counts only changes to this group: newer changes to
# other groups cannot push one out of the read. The window is a validated
# integer, so the query text is constant apart from that number.
$cap = 1000
$xpath = "*[System[(EventID=4732 or EventID=4733) and TimeCreated[timediff(@SystemTime) <= " + ([int64]$hours * 3600000) + "]]] and *[EventData[Data[@Name='TargetSid']='S-1-5-32-544']]"
$events = @()
try { $events = @(Get-WinEvent -LogName 'Security' -FilterXPath $xpath -MaxEvents $cap -ErrorAction Stop) }
catch { if ([string]$_.FullyQualifiedErrorId -notlike 'NoMatchingEventsFound*') { throw } }
foreach ($e in $events) {
  $d = @{}
  foreach ($n in ([xml]$e.ToXml()).Event.EventData.Data) { $d[[string]$n.Name] = [string]$n.'#text' }
  if ($d['TargetSid'] -ne 'S-1-5-32-544') { continue }
  $memberName = $null
  try { $memberName = (New-Object System.Security.Principal.SecurityIdentifier($d['MemberSid'])).Translate([System.Security.Principal.NTAccount]).Value } catch { $memberName = $null }
  $action = 'added'
  if ($e.Id -eq 4733) { $action = 'removed' }
  [void]$changes.Add([ordered]@{
    record_id = [int64]$e.RecordId
    time_created = (Iso $e.TimeCreated)
    action = $action
    member_sid = $d['MemberSid']
    member_name = $memberName
    changed_by_sid = $d['SubjectUserSid']
    changed_by = ($d['SubjectDomainName'] + '\' + $d['SubjectUserName'])
  })
}
$members = @(Admin-Members | ForEach-Object { $_.sid })
Emit-Ok ([ordered]@{ audit_policy = $audit; window_hours = $hours; capped = ($events.Count -ge $cap); changes = $changes.ToArray(); member_sids = $members })
