#!/usr/bin/env pwsh

# Exit immediately if any command fails
$ErrorActionPreference = "Stop"

# Change directory to the parent directory of the script
Set-Location -Path (Split-Path -Parent $PSCommandPath | Split-Path -Parent)

# Set ROOTDIR environment variable to the current directory
$env:ROOTDIR = (Get-Location).Path

# Check if the operating system is macOS
if ($IsMacOS) {
    brew bundle
}

# Source the ensure-go-installed.ps1 script
. ./script/ensure-go-installed.ps1