# Install connector-engine on Windows from a GitHub release, verified against checksums.txt.
#
#   irm https://raw.githubusercontent.com/versatile-sh/connector-engine/main/scripts/install.ps1 | iex
#
# Settings, all optional (environment variables):
#   CONNECTOR_ENGINE_VERSION      a release tag such as v0.1.0; default: the latest release
#   CONNECTOR_ENGINE_PROFILE      person (default: engine, console, cnctl) or headless (engine, cnctl)
#   CONNECTOR_ENGINE_INSTALL_DIR  where the binaries go; default: %LOCALAPPDATA%\connector-engine\bin
#   CONNECTOR_ENGINE_BASE_URL     where the release files are; default: the GitHub release
$ErrorActionPreference = 'Stop'

$repo = 'versatile-sh/connector-engine'
$version = if ($env:CONNECTOR_ENGINE_VERSION) { $env:CONNECTOR_ENGINE_VERSION } else { 'latest' }
$profileName = if ($env:CONNECTOR_ENGINE_PROFILE) { $env:CONNECTOR_ENGINE_PROFILE } else { 'person' }
$installDir = if ($env:CONNECTOR_ENGINE_INSTALL_DIR) { $env:CONNECTOR_ENGINE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'connector-engine\bin' }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  'AMD64' { 'amd64' }
  'ARM64' { 'arm64' }
  default { throw "install.ps1: unsupported architecture $($env:PROCESSOR_ARCHITECTURE)" }
}

switch ($profileName) {
  'person' { $archive = "connector-engine_windows_$arch.zip"; $binaries = @('connector-engine-selfhosted.exe', 'connector-console.exe', 'cnctl.exe') }
  'headless' { $archive = "connector-engine-headless_windows_$arch.zip"; $binaries = @('connector-engine.exe', 'cnctl.exe') }
  default { throw "install.ps1: CONNECTOR_ENGINE_PROFILE must be person or headless, not $profileName" }
}

$baseUrl = if ($env:CONNECTOR_ENGINE_BASE_URL) { $env:CONNECTOR_ENGINE_BASE_URL }
  elseif ($version -eq 'latest') { "https://github.com/$repo/releases/latest/download" }
  else { "https://github.com/$repo/releases/download/$version" }

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("connector-engine-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  Write-Host "Downloading $archive ($version) from $baseUrl"
  Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$archive" -OutFile (Join-Path $tmp $archive)
  Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')

  $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { ($_ -split '\s+')[1] -eq $archive } | Select-Object -First 1
  if (-not $line) { throw "install.ps1: checksums.txt has no entry for $archive" }
  $want = ($line -split '\s+')[0].ToLowerInvariant()
  $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLowerInvariant()
  if ($want -ne $got) { throw "install.ps1: SHA-256 mismatch for ${archive}: expected $want, got $got. Nothing was installed." }
  Write-Host "Verified SHA-256 $got"

  Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath (Join-Path $tmp 'x')
  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  foreach ($b in $binaries) {
    $src = Join-Path $tmp "x\$b"
    if (-not (Test-Path $src)) { throw "install.ps1: $archive has no $b" }
    Copy-Item -Force $src (Join-Path $installDir $b)
    Write-Host "Installed $(Join-Path $installDir $b)"
  }
} finally {
  Remove-Item -Recurse -Force $tmp
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($userPath -split ';') -contains $installDir)) {
  Write-Host ""
  Write-Host "$installDir is not on your PATH. Add it for your user with:"
  Write-Host "  [Environment]::SetEnvironmentVariable('Path', `"$installDir;`$([Environment]::GetEnvironmentVariable('Path','User'))`", 'User')"
}

Write-Host ""
if ($profileName -eq 'person') {
  Write-Host "Next step: start it; it opens the console in your browser."
  Write-Host "  connector-engine-selfhosted up --open"
  Write-Host "The 'up' command ships with the no-Docker runtime, from the next release."
  Write-Host "Until then, use the Docker quick start: https://github.com/$repo/blob/main/docs/getting-started/install.md"
} else {
  Write-Host "Next step: write connector-engine.yaml and adapters.yaml, then run"
  Write-Host "  connector-engine --config connector-engine.yaml --adapters adapters.yaml adapters check"
  Write-Host "Guide: https://github.com/$repo/blob/main/docs/enterprise/headless.md"
}
