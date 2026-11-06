$rows = New-Object System.Collections.ArrayList
foreach ($f in @(Get-WindowsFeature | Where-Object { $_.Installed })) {
  [void]$rows.Add([ordered]@{
    name = (Str $f.Name)
    display_name = (Str $f.DisplayName)
    feature_type = (Str $f.FeatureType)
    install_state = (Str $f.InstallState)
    parent = (Str $f.Parent)
  })
}
Emit-Ok $rows.ToArray()
