# Funções puras usadas por install.ps1/uninstall.ps1, isoladas aqui para poder testar
# sem tocar em processos, registro ou no disco (veja tests/install.Tests.ps1).

function Get-ConfiguredPort {
    param([string]$Addr)
    if ($Addr -match ':(\d+)$') { $Matches[1] } else { '7777' }
}

# Nome do arquivo para onde mover um .exe em uso antes de copiar o novo; evita colidir
# com um .old deixado por uma troca anterior que ainda não foi liberado.
function Get-OldExeName {
    param([string]$Dest)
    $old = "$Dest.old"
    if (Test-Path $old) { $old = "$Dest.old$(Get-Date -Format 'HHmmssfff')" }
    $old
}

function Add-UserPathEntry {
    param([string]$UserPath, [string]$Bin)
    if (($UserPath -split ';') -notcontains $Bin) {
        return ($UserPath.TrimEnd(';'), $Bin) -join ';'
    }
    return $UserPath
}

function Remove-UserPathEntry {
    param([string]$UserPath, [string]$Bin)
    return (($UserPath -split ';' | Where-Object { $_ -and $_ -ne $Bin }) -join ';')
}
