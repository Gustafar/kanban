# Instala o Kanban para rodar em segundo plano: compila, registra a inicialização com o Windows
# (somente para o seu usuário, sem admin), inicia agora e cria o atalho "Kanban" na Área de Trabalho.
# -Source: pasta de onde compilar (ex.: o worktree de um card); a instalação continua em $PSScriptRoot.
# -NoOpen: não abre o navegador (usado na atualização automática ao entrar em revisão).
param([string]$Source = $PSScriptRoot, [switch]$NoOpen)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'install-helpers.ps1')
$root = $PSScriptRoot
$bin = Join-Path $root 'backend\bin'
$server = Join-Path $bin 'kanban-server.exe'
$stage = Join-Path $bin 'staging'
$data = Join-Path $env:APPDATA 'kanban-board'
$port = Get-ConfiguredPort $env:KB_ADDR

# 1. compila numa pasta temporária e valida tudo: o servidor atual segue no ar durante o build,
#    e só é parado depois que sabemos que frontend e backend compilam e geram os 3 executáveis.
New-Item -ItemType Directory -Force $stage | Out-Null
try {
    Push-Location (Join-Path $Source 'frontend')
    if (-not (Test-Path node_modules)) {
        npm install
        if ($LASTEXITCODE -ne 0) { throw "npm install falhou (exit $LASTEXITCODE)" }
    }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build falhou (exit $LASTEXITCODE)" }
    Pop-Location
    Push-Location (Join-Path $Source 'backend')
    go build -o "$stage\kb.exe" .\cmd\kb
    if ($LASTEXITCODE -ne 0) { throw "go build kb.exe falhou (exit $LASTEXITCODE)" }
    go build -o "$stage\cardume.exe" .\cmd\kb
    if ($LASTEXITCODE -ne 0) { throw "go build cardume.exe falhou (exit $LASTEXITCODE)" }
    go build -ldflags '-H=windowsgui' -o "$stage\kanban-server.exe" .\cmd\server
    if ($LASTEXITCODE -ne 0) { throw "go build kanban-server.exe falhou (exit $LASTEXITCODE)" }
    Pop-Location
    foreach ($name in 'kb.exe', 'cardume.exe', 'kanban-server.exe') {
        $p = Join-Path $stage $name
        if (-not (Test-Path $p) -or (Get-Item $p).Length -eq 0) { throw "$name não foi gerado" }
    }
} catch {
    # desfaz os Push-Location pendentes (frontend e/ou backend), sem erro se já não houver nenhum
    try { Pop-Location -ErrorAction Stop } catch {}
    try { Pop-Location -ErrorAction Stop } catch {}
    Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
    Write-Error "Build falhou, nada foi alterado (servidor em execução continua no ar): $_"
}

# 2. troca: para o servidor (só o nosso, pelo nome), confere a porta e copia os executáveis novos.
#    Se algo falhar daqui pra frente, os .exe antigos são restaurados e o servidor antigo volta a subir.
$oldServerWasRunning = [bool](Get-Process -Name 'kanban-server' -ErrorAction SilentlyContinue)
Get-Process -Name 'kanban-server' -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500

function Restart-OldServer {
    if ($oldServerWasRunning -and (Test-Path $server)) {
        Start-Process -FilePath $server -WindowStyle Hidden -ErrorAction SilentlyContinue
    }
}

$busy = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
if ($busy) {
    $owner = (Get-Process -Id $busy[0].OwningProcess -ErrorAction SilentlyContinue).ProcessName
    Restart-OldServer
    Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
    Write-Error "A porta $port está em uso por '$owner'. Se for o start.ps1 antigo, feche aquele terminal (Ctrl+C) e rode de novo."
}
# um .exe em uso não pode ser sobrescrito, mas pode ser renomeado: o antigo vira .old
# (um .old ainda em uso, ex.: kb.exe mcp de uma execução, não some: o próximo ganha outro nome)
Get-ChildItem $bin -Filter '*.old*' -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
$moved = @()
try {
    foreach ($f in Get-ChildItem $stage -Filter '*.exe') {
        $dest = Join-Path $bin $f.Name
        if (Test-Path $dest) {
            $oldName = Get-OldExeName $dest
            Move-Item -Force $dest $oldName
            $moved += [pscustomobject]@{ Dest = $dest; Old = $oldName }
        }
        Move-Item $f.FullName $dest
    }
} catch {
    Write-Host "Falha ao trocar os executáveis, restaurando a versão anterior..." -ForegroundColor Yellow
    foreach ($m in $moved) {
        Remove-Item $m.Dest -Force -ErrorAction SilentlyContinue
        Move-Item -Force $m.Old $m.Dest -ErrorAction SilentlyContinue
    }
    Restart-OldServer
    Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
    Write-Error "Erro ao trocar os executáveis, versão anterior restaurada e servidor religado: $_"
}
Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue

# 3. inicia com o Windows (chave Run do usuário atual)
Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'KanbanBoard' -Value "`"$server`""

# 3b. o comando `cardume` em qualquer terminal novo: coloca backend\bin no PATH do usuário
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
[Environment]::SetEnvironmentVariable('Path', (Add-UserPathEntry $userPath $bin), 'User')

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
