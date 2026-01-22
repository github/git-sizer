# This script is meant to be sourced with ROOTDIR set.

if (-not $env:ROOTDIR) {
    $env:ROOTDIR = (Resolve-Path (Join-Path $scriptDir "..")).Path
}

# Function to check if Go is installed and at least version 1.21
function GoOk {
    $goVersionOutput = & go version 2>$null
    if ($goVersionOutput) {
        $goVersion = $goVersionOutput -match 'go(\d+)\.(\d+)' | Out-Null
        $majorVersion = [int]$Matches[1]
        $minorVersion = [int]$Matches[2]
        return ($majorVersion -eq 1 -and $minorVersion -ge 21)
    }
    return $false
}

# Function to set up a local Go installation if available
function SetUpVendoredGo {
    $GO_VERSION = "go1.23.7"
    $VENDORED_GOROOT = Join-Path -Path $env:ROOTDIR -ChildPath "vendor/$GO_VERSION/go"
    if (Test-Path -Path "$VENDORED_GOROOT/bin/go") {
        $env:GOROOT = $VENDORED_GOROOT
        $env:PATH = "$env:GOROOT/bin;$env:PATH"
    }
}

# Function to check if Make is installed and install it if needed
function EnsureMakeInstalled {
    $makeInstalled = $null -ne (Get-Command "make" -ErrorAction SilentlyContinue)
    if (-not $makeInstalled) {
        #Write-Host "Installing Make using winget..."
        winget install --no-upgrade --nowarn -e --id GnuWin32.Make 
        if ($LASTEXITCODE -ne 0 -and $LASTEXITCODE -ne 0x8A150061) {
            Write-Error "Failed to install Make. Please install it manually. Exit code: $LASTEXITCODE"
        }
        # Refresh PATH to include the newly installed Make
        $env:PATH = [System.Environment]::GetEnvironmentVariable("PATH", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("PATH", "User")
    }

    # Add GnuWin32 bin directory directly to the PATH
    $gnuWin32Path = "C:\Program Files (x86)\GnuWin32\bin"
    if (Test-Path -Path $gnuWin32Path) {
        $env:PATH = "$gnuWin32Path;$env:PATH"
    } else {
        Write-Host "Couldn't find GnuWin32 bin directory at the expected location."
        # Also refresh PATH from environment variables as a fallback
        $env:PATH = [System.Environment]::GetEnvironmentVariable("PATH", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("PATH", "User")
    }    
}

SetUpVendoredGo

if (-not (GoOk)) {
    & ./script/install-vendored-go >$null
    if ($LASTEXITCODE -ne 0) {
        exit 1
    }
    SetUpVendoredGo
}

# Ensure Make is installed
EnsureMakeInstalled
