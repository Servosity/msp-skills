$rows = New-Object System.Collections.ArrayList
foreach ($v in @(Get-Volume | Where-Object { [string]$_.DriveType -eq 'Fixed' -and $_.Size -gt 0 })) {
  $letter = $null
  if ($v.DriveLetter) { $letter = [string]$v.DriveLetter }
  [void]$rows.Add([ordered]@{
    drive_letter = $letter
    unique_id = (Str $v.UniqueId)
    label = (Str $v.FileSystemLabel)
    file_system = (Str $v.FileSystem)
    size_bytes = [int64]$v.Size
    free_bytes = [int64]$v.SizeRemaining
    percent_free = [math]::Round(100.0 * $v.SizeRemaining / $v.Size, 1)
    health_status = (Str $v.HealthStatus)
  })
}
Emit-Ok $rows.ToArray()
