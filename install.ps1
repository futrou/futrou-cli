#!/usr/bin/env pwsh
param(
  [String]$Version = "latest",
  [Switch]$NoPathUpdate = $false,
  [Switch]$IgnoreSignature = $false,
  [Switch]$DownloadWithoutCurl = $false
)

$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Colors
# ---------------------------------------------------------------------------
$C_RESET  = [char]27 + "[0m"
$C_GREEN  = [char]27 + "[1;32m"
$C_YELLOW = [char]27 + "[0;33m"
$C_DIM    = [char]27 + "[2m"
$C_RED    = [char]27 + "[0;31m"

function Write-Info   { param($msg) Write-Output "${C_DIM}${msg}${C_RESET}" }
function Write-Success{ param($msg) Write-Output "${C_GREEN}${msg}${C_RESET}" }
function Write-Warn   { param($msg) Write-Output "${C_YELLOW}${msg}${C_RESET}" }
function Write-Fail   { param($msg) Write-Output "${C_RED}error: ${msg}${C_RESET}"; exit 1 }

# ---------------------------------------------------------------------------
# Detect architecture from registry (reliable under ARM64 emulation)
# ---------------------------------------------------------------------------
$Arch = (Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment').PROCESSOR_ARCHITECTURE

if (-not ($Arch -eq "AMD64" -or $Arch -eq "ARM64")) {
  Write-Fail "Futrou CLI only supports x86_64 and ARM64 Windows."
}

$GoArch = if ($Arch -eq "ARM64") { "arm64" } else { "amd64" }
$Target = "windows-$GoArch"

# ---------------------------------------------------------------------------
# Resolve version
# ---------------------------------------------------------------------------
if ($Version -match "^\d+\.\d+\.\d+$")  { $Version = "v$Version" }

$BaseURL  = "https://github.com/futrou/futrou-cli/releases"
$GpgKeyURL = "https://futrou.com/.well-known/pgp-key.txt"
$FileName = "futrou-$Target.exe"
$URL = if ($Version -eq "latest") {
  "$BaseURL/latest/download/$FileName"
} else {
  "$BaseURL/download/$Version/$FileName"
}

# ---------------------------------------------------------------------------
# Install location  (%USERPROFILE%\.futrou\bin\futrou.exe)
# ---------------------------------------------------------------------------
$InstallRoot = if ($env:FUTROU_INSTALL) { $env:FUTROU_INSTALL } else { "$HOME\.futrou" }
$BinDir      = "$InstallRoot\bin"
$Exe         = "$BinDir\futrou.exe"

$null = New-Item -ItemType Directory -Force -Path $BinDir

# ---------------------------------------------------------------------------
# Detect existing installation and decide action label
# ---------------------------------------------------------------------------
$Action = "Installing"
$CurrentVersion = $null

if (Test-Path $Exe) {
  try {
    $raw = & $Exe version 2>$null
    if ($raw -match '(\d+\.\d+\.\d+)') { $CurrentVersion = $Matches[1] }
  } catch { }
}

if ($CurrentVersion -and $Version -ne "latest") {
  $TargetVersion = $Version.TrimStart('v')
  if ($CurrentVersion -eq $TargetVersion) {
    Write-Info "Futrou CLI v$CurrentVersion is already installed at $Exe"
    exit 0
  }
  $cur = [Version]$CurrentVersion
  $tgt = [Version]$TargetVersion
  $Action = if ($tgt -gt $cur) { "Upgrading" } elseif ($tgt -lt $cur) { "Downgrading" } else { "Reinstalling" }
} elseif ($CurrentVersion) {
  $Action = "Upgrading"
}

$DisplayVersion = if ($Version -eq "latest") { "latest" } else { $Version.TrimStart('v') }

$DisplayVersionLabel = if ($Version -eq "latest") { "latest" } else { "v$DisplayVersion" }

if ($CurrentVersion) {
  Write-Info "$Action Futrou CLI v$CurrentVersion -> $DisplayVersionLabel"
} else {
  Write-Info "Installing Futrou CLI $DisplayVersionLabel"
}

# ---------------------------------------------------------------------------
# Download
# ---------------------------------------------------------------------------
$TmpExe = "$BinDir\futrou-tmp.exe"
Remove-Item -Force $TmpExe -ErrorAction SilentlyContinue

$downloaded = $false

if (-not $DownloadWithoutCurl) {
  try {
    curl.exe "-#SfLo" $TmpExe $URL
    if ($LASTEXITCODE -eq 0) { $downloaded = $true }
  } catch { }
}

if (-not $downloaded) {
  try {
    Invoke-RestMethod -Uri $URL -OutFile $TmpExe
    $downloaded = $true
  } catch {
    if ($Version -eq "latest") {
      Write-Fail "Failed to download latest release. Try again later.`n  $URL"
    } else {
      Write-Fail "Version $Version not found or binary not available for $Target.`n  $URL"
    }
  }
}

if (-not (Test-Path $TmpExe)) {
  Write-Fail "Download produced no file. Did antivirus delete it?"
}

# ---------------------------------------------------------------------------
# Verify the PGP signature (<file>.asc next to the binary). Needs gpg; without
# it only a warning is shown. A private keyring is used, never the user's.
# ---------------------------------------------------------------------------
function Fail-Signature([String]$Reason) {
  Remove-Item -Force $TmpExe -ErrorAction SilentlyContinue
  $Args2 = if ($Version -eq "latest") { "-IgnoreSignature" } else { "-Version $Version -IgnoreSignature" }
  Write-Warn "warning: could not verify the signature: $Reason"
  Write-Info "Nothing was installed. If you trust the download anyway, run the installer again without the check:"
  Write-Output "  & ([scriptblock]::Create((irm https://raw.githubusercontent.com/futrou/futrou-cli/main/install.ps1))) $Args2"
  exit 1
}

if ($IgnoreSignature) {
  Write-Warn "Skipping signature verification (-IgnoreSignature)"
} elseif (-not (Get-Command gpg -ErrorAction SilentlyContinue)) {
  Write-Warn "gpg is not installed, skipping signature verification."
  Write-Info "Install gnupg and run the installer again to verify releases against $GpgKeyURL"
} else {
  $GpgHome = Join-Path ([IO.Path]::GetTempPath()) ("futrou-gpg-" + [Guid]::NewGuid())
  $null = New-Item -ItemType Directory -Force -Path $GpgHome
  try {
    try { Invoke-WebRequest -UseBasicParsing -Uri $GpgKeyURL -OutFile "$GpgHome\pgp-key.txt" } catch { Fail-Signature "failed to connect to $GpgKeyURL to download the public key" }
    & gpg --homedir $GpgHome --batch --quiet --import "$GpgHome\pgp-key.txt" 2>$null
    if ($LASTEXITCODE -ne 0) { Fail-Signature "the downloaded public key is not valid" }
    try { Invoke-WebRequest -UseBasicParsing -Uri "$URL.asc" -OutFile "$GpgHome\futrou.asc" } catch { Fail-Signature "failed to download the signature from $URL.asc" }
    $Status = & gpg --homedir $GpgHome --batch --no-tty --status-fd 1 --verify "$GpgHome\futrou.asc" $TmpExe 2>$null
    if ($LASTEXITCODE -ne 0) { Fail-Signature "the signature does not match the downloaded file" }
    if (-not ($Status -match '^\[GNUPG:\] VALIDSIG ')) { Fail-Signature "no valid signature on the downloaded file" }
    if ($Status -match '^\[GNUPG:\] (EXPKEYSIG|EXPSIG|REVKEYSIG) ') { Fail-Signature "the signing key is expired or revoked" }
    Write-Info "Signature verified"
  } finally {
    Remove-Item -Recurse -Force $GpgHome -ErrorAction SilentlyContinue
  }
}

try { Remove-Item -Force $Exe -ErrorAction SilentlyContinue } catch { }
Move-Item -Force $TmpExe $Exe

# ---------------------------------------------------------------------------
# Verify
# ---------------------------------------------------------------------------
$InstalledVersion = $null
try {
  $raw = & $Exe version 2>$null
  if ($raw -match '(\d+\.\d+\.\d+)') { $InstalledVersion = $Matches[1] }
} catch { }

if (-not $InstalledVersion) {
  Write-Fail "Installed binary did not run correctly."
}

$ActionPast = switch ($Action) {
  "Installing"  { "installed" }
  "Upgrading"   { "upgraded" }
  "Downgrading" { "downgraded" }
  default       { "installed" }
}

Write-Success "Futrou CLI v$InstalledVersion was $ActionPast to $Exe"

# ---------------------------------------------------------------------------
# PATH update
# ---------------------------------------------------------------------------
function Get-UserPath {
  $key = (Get-Item 'HKCU:').OpenSubKey('Environment')
  $key.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
}

function Set-UserPath([String]$Value) {
  $key = (Get-Item 'HKCU:').OpenSubKey('Environment', $true)
  $kind = if ($Value.Contains('%')) {
    [Microsoft.Win32.RegistryValueKind]::ExpandString
  } else {
    [Microsoft.Win32.RegistryValueKind]::String
  }
  $key.SetValue('Path', $Value, $kind)

  if (-not ("Win32.NativeMethods" -as [Type])) {
    Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition @"
[DllImport("user32.dll", SetLastError=true, CharSet=CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam,
    string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
"@
  }
  $result = [UIntPtr]::Zero
  [Win32.NativeMethods]::SendMessageTimeout(
    [IntPtr]0xffff, 0x1a, [UIntPtr]::Zero, "Environment", 2, 5000, [ref]$result
  ) | Out-Null
}

if (-not $NoPathUpdate) {
  $CurrentPath = (Get-UserPath) -split ';' | Where-Object { $_ -ne '' }
  if ($CurrentPath -notcontains $BinDir) {
    $NewPath = ($CurrentPath + $BinDir) -join ';'
    Set-UserPath $NewPath
    $env:PATH = $env:PATH + ";$BinDir"
    Write-Info "Added $BinDir to your PATH"
  }
}

# Make futrou available in the current session without restarting
if ($env:PATH -notlike "*$BinDir*") {
  $env:PATH = "$BinDir;$env:PATH"
}

Write-Output ""
Write-Output "To get started, run: futrou --help"
