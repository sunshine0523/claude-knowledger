$ErrorActionPreference = "Stop"

$Repo = "sunshine0523/claude-knowledger"
$Binary = "knowledger.exe"

$Arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }

$Response = Invoke-WebRequest "https://github.com/$Repo/releases/latest" -MaximumRedirection 0 -ErrorAction SilentlyContinue
$Version = ($Response.Headers.Location -split '/v')[-1]
$Archive = "claude-knowledger_${Version}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/v${Version}/$Archive"

Write-Host "Installing knowledger v$Version (windows/$Arch)..."

$Tmp = Join-Path $env:TEMP ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $Tmp | Out-Null

$ZipPath = Join-Path $Tmp $Archive
Invoke-WebRequest $Url -OutFile $ZipPath
Expand-Archive $ZipPath -DestinationPath $Tmp

$InstallDir = "$env:USERPROFILE\.local\bin"
if (-not (Test-Path $InstallDir)) { New-Item -ItemType Directory -Path $InstallDir | Out-Null }

Move-Item (Join-Path $Tmp $Binary) (Join-Path $InstallDir $Binary) -Force
Remove-Item $Tmp -Recurse -Force

# Add to PATH for current user if not already there
$CurrentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($CurrentPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$CurrentPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to PATH (restart terminal to take effect)"
}

Write-Host "Installed to $InstallDir\$Binary"
