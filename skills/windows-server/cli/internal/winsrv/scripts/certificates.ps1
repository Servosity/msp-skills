$store = Need-Enum 'store' ([string](Arg 'store' 'My')) @('My', 'WebHosting', 'Remote Desktop', 'CA', 'Root')
$path = 'Cert:\LocalMachine\' + $store
if (-not (Test-Path -LiteralPath $path)) { Emit-Ok @() }
$rows = New-Object System.Collections.ArrayList
foreach ($c in @(Get-ChildItem -LiteralPath $path)) {
  [void]$rows.Add([ordered]@{
    store = $store
    thumbprint = (Str $c.Thumbprint)
    subject = (Str $c.Subject)
    issuer = (Str $c.Issuer)
    # Not Iso: its pre-2000 cutoff is for "never ran" sentinels, and a root
    # certificate can genuinely date from the 1990s.
    not_before = $c.NotBefore.ToUniversalTime().ToString('o')
    not_after = $c.NotAfter.ToUniversalTime().ToString('o')
    has_private_key = [bool]$c.HasPrivateKey
    dns_names = @($c.DnsNameList | ForEach-Object { [string]$_.Unicode } | Where-Object { $_ })
  })
}
Emit-Ok $rows.ToArray()
