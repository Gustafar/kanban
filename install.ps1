# Instala o Kanban para rodar em segundo plano: compila, registra a inicialização com o Windows
# (somente para o seu usuário, sem admin), inicia agora e cria o atalho "Kanban" na Área de Trabalho.
# -Source: pasta de onde compilar (ex.: o worktree de um card); a instalação continua em $PSScriptRoot.
# -NoOpen: não abre o navegador (usado na atualização automática ao entrar em revisão).
param([string]$Source = $PSScriptRoot, [switch]$NoOpen)
$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$bin = Join-Path $root 'backend\bin'
$server = Join-Path $bin 'kanban-server.exe'
$stage = Join-Path $bin 'staging'
$data = Join-Path $env:APPDATA 'kanban-board'
$port = if ($env:KB_ADDR -match ':(\d+)$') { $Matches[1] } else { '7777' }

# 1. compila numa pasta temporária: o servidor atual segue no ar durante o build
New-Item -ItemType Directory -Force $stage | Out-Null
Push-Location (Join-Path $Source 'frontend')
if (-not (Test-Path node_modules)) { npm install }
npm run build
Pop-Location
Push-Location (Join-Path $Source 'backend')
go build -o "$stage\kb.exe" .\cmd\kb
go build -o "$stage\cardume.exe" .\cmd\kb
go build -ldflags '-H=windowsgui' -o "$stage\kanban-server.exe" .\cmd\server
Pop-Location

# 2. troca: para o servidor (só o nosso, pelo nome), confere a porta e copia os executáveis novos
Get-Process -Name 'kanban-server' -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500
$busy = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
if ($busy) {
    $owner = (Get-Process -Id $busy[0].OwningProcess -ErrorAction SilentlyContinue).ProcessName
    Write-Error "A porta $port está em uso por '$owner'. Se for o start.ps1 antigo, feche aquele terminal (Ctrl+C) e rode de novo."
}
# um .exe em uso não pode ser sobrescrito, mas pode ser renomeado: o antigo vira .old
# (um .old ainda em uso, ex.: kb.exe mcp de uma execução, não some: o próximo ganha outro nome)
Get-ChildItem $bin -Filter '*.old*' -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
foreach ($f in Get-ChildItem $stage -Filter '*.exe') {
    $dest = Join-Path $bin $f.Name
    if (Test-Path $dest) {
        $old = "$dest.old"
        if (Test-Path $old) { $old = "$dest.old$(Get-Date -Format 'HHmmssfff')" }
        Move-Item -Force $dest $old
    }
    Move-Item $f.FullName $dest
}
Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue

# 3. inicia com o Windows (chave Run do usuário atual)
Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'KanbanBoard' -Value "`"$server`""

# 3b. o comando `cardume` em qualquer terminal novo: coloca backend\bin no PATH do usuário
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $bin) {
    [Environment]::SetEnvironmentVariable('Path', (($userPath.TrimEnd(';'), $bin) -join ';'), 'User')
}

# 4. inicia agora, sem janela
Start-Process -FilePath $server -WindowStyle Hidden
$tokenFile = Join-Path $data 'token'
for ($i = 0; $i -lt 40 -and -not (Test-Path $tokenFile); $i++) { Start-Sleep -Milliseconds 250 }
if (-not (Test-Path $tokenFile)) { Write-Error "O servidor não iniciou. Veja $data\server.log" }
$token = (Get-Content $tokenFile -Raw).Trim()
$url = "http://localhost:$port/?token=$token"

# 5. atalhos: Área de Trabalho e Menu Iniciar. Com Chrome/Edge, abrem numa janela própria (modo app,
#    sem barra de endereço); sem eles, caem no navegador padrão (.url).
$browser = @(
    "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
    "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
    "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe",
    "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
    "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe"
) | Where-Object { Test-Path $_ } | Select-Object -First 1
$shell = New-Object -ComObject WScript.Shell
foreach ($dir in @([Environment]::GetFolderPath('Desktop'), (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'))) {
    Remove-Item (Join-Path $dir 'Kanban.url'), (Join-Path $dir 'Kanban.lnk') -ErrorAction SilentlyContinue
    if ($browser) {
        $lnk = $shell.CreateShortcut((Join-Path $dir 'Kanban.lnk'))
        $lnk.TargetPath = $browser
        $lnk.Arguments = "--app=$url"
        $lnk.IconLocation = "$browser,0"
        $lnk.Save()
    } else {
        Set-Content -Path (Join-Path $dir 'Kanban.url') -Value "[InternetShortcut]`r`nURL=$url" -Encoding ASCII
    }
}

Write-Host ""
Write-Host "Pronto. O Kanban agora sobe sozinho quando você entrar no Windows." -ForegroundColor Green
Write-Host "Atalho 'Kanban' criado na Área de Trabalho e no Menu Iniciar."
Write-Host "Em qualquer pasta, num terminal novo: cardume   (abre o board no projeto daquela pasta)"
Write-Host "Logs: $data\server.log"
Write-Host "Para remover: .\uninstall.ps1"
if (-not $NoOpen) {
    if ($browser) { Start-Process -FilePath $browser -ArgumentList "--app=$url" } else { Start-Process $url }
}
