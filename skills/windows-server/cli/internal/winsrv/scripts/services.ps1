$rows = New-Object System.Collections.ArrayList
foreach ($s in @(Get-CimInstance -ClassName Win32_Service)) { [void]$rows.Add((Service-Row $s)) }
Emit-Ok $rows.ToArray()
