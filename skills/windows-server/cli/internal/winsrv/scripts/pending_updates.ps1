# Read-only: Search only. The Windows Update Agent never downloads or installs here.
$session = New-Object -ComObject Microsoft.Update.Session
$result = $session.CreateUpdateSearcher().Search("IsInstalled=0 and IsHidden=0 and Type='Software'")
$rows = New-Object System.Collections.ArrayList
foreach ($u in @($result.Updates)) {
  [void]$rows.Add([ordered]@{
    update_id = (Str $u.Identity.UpdateID)
    title = (Str $u.Title)
    kb_ids = @($u.KBArticleIDs | ForEach-Object { 'KB' + [string]$_ })
    severity = (Str $u.MsrcSeverity)
    categories = @($u.Categories | ForEach-Object { [string]$_.Name })
    is_downloaded = [bool]$u.IsDownloaded
    max_download_bytes = [int64]$u.MaxDownloadSize
  })
}
Emit-Ok $rows.ToArray()
