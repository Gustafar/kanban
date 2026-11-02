# Builds the UI and the Go binaries, then starts the server (prints the access URLs with token).
$ErrorActionPreference = 'Stop'
Push-Location "$PSScriptRoot\frontend"
if (-not (Test-Path node_modules)) { npm install }
npm run build
Pop-Location
Push-Location "$PSScriptRoot\backend"
go build -o bin\kb.exe .\cmd\kb
go build -o bin\server.exe .\cmd\server
Pop-Location
& "$PSScriptRoot\backend\bin\server.exe"
