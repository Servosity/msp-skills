$rows = New-Object System.Collections.ArrayList
foreach ($s in @(Get-SmbShare)) {
  [void]$rows.Add([ordered]@{
    name = (Str $s.Name)
    path = (Str $s.Path)
    description = (Str $s.Description)
    special = [bool]$s.Special
    current_users = [int64]$s.CurrentUsers
    encrypt_data = [bool]$s.EncryptData
  })
}
Emit-Ok $rows.ToArray()
