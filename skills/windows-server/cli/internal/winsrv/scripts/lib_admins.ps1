# Admin-Members lists the builtin Administrators group (SID S-1-5-32-544) by SID.
# Get-LocalGroupMember fails on an orphaned SID ("Failed to compare two
# elements"); the ADSI fallback reads the same members without resolving them.
function Admin-Members {
  $rows = New-Object System.Collections.ArrayList
  try {
    foreach ($m in @(Get-LocalGroupMember -SID 'S-1-5-32-544')) {
      [void]$rows.Add([ordered]@{ name = (Str $m.Name); object_class = (Str $m.ObjectClass); principal_source = (Str $m.PrincipalSource); sid = (Str $m.SID.Value) })
    }
    return $rows.ToArray()
  } catch { $rows.Clear() }
  $sid = New-Object System.Security.Principal.SecurityIdentifier('S-1-5-32-544')
  $groupName = ($sid.Translate([System.Security.Principal.NTAccount]).Value -split '\\')[-1]
  $group = [ADSI]('WinNT://' + $env:COMPUTERNAME + '/' + $groupName + ',group')
  foreach ($m in @($group.psbase.Invoke('Members'))) {
    $bytes = $m.GetType().InvokeMember('objectSid', 'GetProperty', $null, $m, $null)
    $msid = (New-Object System.Security.Principal.SecurityIdentifier($bytes, 0)).Value
    $name = $null
    try { $name = (New-Object System.Security.Principal.SecurityIdentifier($msid)).Translate([System.Security.Principal.NTAccount]).Value } catch { $name = $null }
    $class = [string]$m.GetType().InvokeMember('Class', 'GetProperty', $null, $m, $null)
    [void]$rows.Add([ordered]@{ name = $name; object_class = $class; principal_source = $null; sid = $msid })
  }
  return $rows.ToArray()
}
