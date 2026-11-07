if ($null -eq (Get-Service -Name vmms -ErrorAction SilentlyContinue)) { Emit-Err 'role_absent' 'The Hyper-V role is not installed' }
if ($null -eq (Get-Command Hyper-V\Get-VM -ErrorAction SilentlyContinue)) { Emit-Err 'unavailable' 'The Hyper-V role is installed but its PowerShell module is not (Hyper-V-PowerShell feature)' }
$rows = New-Object System.Collections.ArrayList
# Module-qualified names so another vendor's Get-VM cannot shadow Hyper-V's.
foreach ($vm in @(Hyper-V\Get-VM)) {
  $snaps = @(); $snapErr = $null
  try { $snaps = @(Hyper-V\Get-VMSnapshot -VM $vm -ErrorAction Stop) } catch { $snapErr = [string]$_.Exception.Message }
  $oldest = $null
  foreach ($c in $snaps) { if ($null -eq $oldest -or $c.CreationTime -lt $oldest) { $oldest = $c.CreationTime } }
  $oldestDays = $null
  if ($null -ne $oldest) { $oldestDays = [int64]((Get-Date) - $oldest).TotalDays }
  [void]$rows.Add([ordered]@{
    name = (Str $vm.Name)
    vm_id = (Str $vm.VMId)
    state = (Str $vm.State)
    uptime_seconds = [int64]$vm.Uptime.TotalSeconds
    memory_assigned_bytes = [int64]$vm.MemoryAssigned
    generation = [int]$vm.Generation
    version = (Str $vm.Version)
    checkpoint_count = $snaps.Count
    checkpoint_error = $snapErr
    oldest_checkpoint_days = $oldestDays
    replication_state = (Str $vm.ReplicationState)
    replication_health = (Str $vm.ReplicationHealth)
  })
}
Emit-Ok $rows.ToArray()
