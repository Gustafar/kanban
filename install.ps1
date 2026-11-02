# Instala o Kanban para rodar em segundo plano: compila, registra a inicialização com o Windows
# (somente para o seu usuário, sem admin), inicia agora e cria o atalho "Kanban" na Área de Trabalho.
$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$bin = Join-Path $root 'backend\bin'
$server = Join-Path $bin 'kanban-server.exe'
$data = Join-Path $env:APPDATA 'kanban-board'
$port = if ($env:KB_ADDR -match ':(\d+)$') { $Matches[1] } else { '7777' }

# 1. libera o executável (só o nosso, pelo nome) e avisa se a porta estiver com outro processo
Get-Process -Name 'kanban-server' -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500
$busy = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
if ($busy) {
    $owner = (Get-Process -Id $busy[0].OwningProcess -ErrorAction SilentlyContinue).ProcessName
    Write-Error "A porta $port está em uso por '$owner'. Se for o start.ps1 antigo, feche aquele terminal (Ctrl+C) e rode de novo."
}

# 2. compila
Push-Location (Join-Path $root 'frontend')
if (-not (Test-Path node_modules)) { npm install }
npm run build
Pop-Location
Push-Location (Join-Path $root 'backend')
go build -o bin\kb.exe .\cmd\kb
go build -ldflags '-H=windowsgui' -o bin\kanban-server.exe .\cmd\server
Pop-Location

# 3. inicia com o Windows (chave Run do usuário atual)
Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'KanbanBoard' -Value "`"$server`""

# 4. inicia agora, sem janela
Start-Process -FilePath $server -WindowStyle Hidden
$tokenFile = Join-Path $data 'token'
for ($i = 0; $i -lt 40 -and -not (Test-Path $tokenFile); $i++) { Start-Sleep -Milliseconds 250 }
if (-not (Test-Path $tokenFile)) { Write-Error "O servidor não iniciou. Veja $data\server.log" }
$token = (Get-Content $tokenFile -Raw).Trim()
$url = "http://localhost:$port/?token=$token"

# 5. atalhos: Área de Trabalho e Menu Iniciar (arquivo .url, abre no navegador padrão)
foreach ($dir in @([Environment]::GetFolderPath('Desktop'), (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'))) {
    Set-Content -Path (Join-Path $dir 'Kanban.url') -Value "[InternetShortcut]`r`nURL=$url" -Encoding ASCII
}

Write-Host ""
Write-Host "Pronto. O Kanban agora sobe sozinho quando você entrar no Windows." -ForegroundColor Green
Write-Host "Atalho 'Kanban' criado na Área de Trabalho e no Menu Iniciar."
Write-Host "Ou rode: backend\bin\kb.exe open"
Write-Host "Logs: $data\server.log"
Write-Host "Para remover: .\uninstall.ps1"
Start-Process $url
