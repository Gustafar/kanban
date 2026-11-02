# Testes das funções puras usadas por install.ps1/uninstall.ps1 (fluxo do instalador).
# Rodar com: Invoke-Pester (na raiz do repo, ou apontando para esta pasta).

. (Join-Path $PSScriptRoot '..\install-helpers.ps1')

Describe 'Get-ConfiguredPort' {
    It 'usa a porta do KB_ADDR quando presente' {
        Get-ConfiguredPort ':8080' | Should Be '8080'
    }
    It 'aceita host:porta' {
        Get-ConfiguredPort '0.0.0.0:9000' | Should Be '9000'
    }
    It 'cai para 7777 quando KB_ADDR está vazio' {
        Get-ConfiguredPort '' | Should Be '7777'
    }
    It 'cai para 7777 quando KB_ADDR não tem porta' {
        Get-ConfiguredPort 'localhost' | Should Be '7777'
    }
}

Describe 'Get-OldExeName' {
    It 'usa .old quando não há colisão' {
        Mock Test-Path { $false }
        Get-OldExeName 'C:\bin\kb.exe' | Should Be 'C:\bin\kb.exe.old'
    }
    It 'acrescenta um timestamp quando .old já existe' {
        Mock Test-Path { $true }
        $name = Get-OldExeName 'C:\bin\kb.exe'
        $name | Should Not Be 'C:\bin\kb.exe.old'
        $name | Should Match '^C:\\bin\\kb\.exe\.old\d+$'
    }
}

Describe 'Add-UserPathEntry' {
    It 'acrescenta a pasta quando ainda não está no PATH' {
        Add-UserPathEntry 'C:\a;C:\b' 'C:\bin' | Should Be 'C:\a;C:\b;C:\bin'
    }
    It 'não duplica quando a pasta já está no PATH' {
        Add-UserPathEntry 'C:\a;C:\bin;C:\b' 'C:\bin' | Should Be 'C:\a;C:\bin;C:\b'
    }
    It 'lida com um PATH terminado em ; sem gerar entrada vazia' {
        Add-UserPathEntry 'C:\a;' 'C:\bin' | Should Be 'C:\a;C:\bin'
    }
}

Describe 'Remove-UserPathEntry' {
    It 'remove a pasta quando presente' {
        Remove-UserPathEntry 'C:\a;C:\bin;C:\b' 'C:\bin' | Should Be 'C:\a;C:\b'
    }
    It 'não altera o PATH quando a pasta não está presente' {
        Remove-UserPathEntry 'C:\a;C:\b' 'C:\bin' | Should Be 'C:\a;C:\b'
    }
    It 'descarta entradas vazias' {
        Remove-UserPathEntry 'C:\a;;C:\bin' 'C:\bin' | Should Be 'C:\a'
    }
}
