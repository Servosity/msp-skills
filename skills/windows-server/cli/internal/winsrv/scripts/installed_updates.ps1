$rows = New-Object System.Collections.ArrayList
foreach ($h in @(Get-HotFix)) {
  $on = $null
  try { $on = Iso $h.InstalledOn } catch { $on = $null }
  [void]$rows.Add([ordered]@{
    hotfix_id = (Str $h.HotFixID)
    description = (Str $h.Description)
    installed_on = $on
    installed_by = (Str $h.InstalledBy)
  })
}
Emit-Ok $rows.ToArray()
