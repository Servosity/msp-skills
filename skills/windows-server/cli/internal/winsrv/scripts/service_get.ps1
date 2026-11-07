$n = Need-Name 'name' (Arg 'name' '')
$hit = @(Get-CimInstance -ClassName Win32_Service | Where-Object { [string]$_.Name -eq $n })
if ($hit.Count -eq 0) { Emit-Err 'not_found' 'No service with that name' }
Emit-Ok (Service-Row $hit[0])
