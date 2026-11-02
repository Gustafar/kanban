# Remove a inicialização automática, para o servidor e apaga os atalhos. Seus dados (cards, projetos) são mantidos.
$ErrorActionPreference = 'Continue'
. (Join-Path $PSScriptRoot 'install-helpers.ps1')
Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'KanbanBoard' -ErrorAction SilentlyContinue
Get-Process -Name 'kanban-server' -ErrorAction SilentlyContinue | Stop-Process -Force
foreach ($dir in @([Environment]::GetFolderPath('Desktop'), (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'))) {
    Remove-Item (Join-Path $dir 'Kanban.url'), (Join-Path $dir 'Kanban.lnk') -ErrorAction SilentlyContinue
}
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$bin = Join-Path $PSScriptRoot 'backend\bin'
[Environment]::SetEnvironmentVariable('Path', (Remove-UserPathEntry $userPath $bin), 'User')
Write-Host "Removido. Seus dados continuam em $env:APPDATA\kanban-board (apague a pasta se quiser zerar tudo)."
