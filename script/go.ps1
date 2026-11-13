# Ensure that script errors stop execution
$ErrorActionPreference = "Stop"

# Determine the root directory of the project.
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ROOTDIR = (Resolve-Path (Join-Path $scriptDir "..")).Path

# Source the ensure-go-installed functionality.
# (This assumes you have a corresponding PowerShell version of ensure-go-installed.
#  If not, you could call the bash version via bash.exe if available.)
$ensureScript = Join-Path $ROOTDIR "script\ensure-go-installed.ps1"
if (Test-Path $ensureScript) {
    . $ensureScript
} else {
    Write-Error "Unable to locate '$ensureScript'. Please provide a PowerShell version of ensure-go-installed."
}

# Execute the actual 'go' command with passed arguments.
# This re-invokes the Go tool in PATH.
$goExe = "go"
& $goExe @args