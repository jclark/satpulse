# Build satpulsetool for Windows. Analogous to bsd-build.sh.

$ErrorActionPreference = 'Stop'

$env:GOOS = 'windows'

# Detect or validate GOARCH
if (-not $env:GOARCH) {
    $env:GOARCH = switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "Unsupported architecture $env:PROCESSOR_ARCHITECTURE" }
    }
}
if ($env:GOARCH -notin @('amd64', 'arm64')) {
    throw "Unsupported GOARCH $env:GOARCH"
}

# Force UTC so git date formatting matches bsd-build.sh.
$env:TZ = 'UTC'

# Build info
$version = (Get-Content VERSION -Raw).Trim()
$build_date = [DateTime]::UtcNow.ToString('yyyy-MM-dd HH:mm:ss+00:00')
$git_date = (git log -1 '--format=%cd' '--date=format-local:%Y%m%d').Trim()
$git_hash = (git log -1 '--format=%h').Trim()

$cmd_version = "$version-pre.$git_date.$git_hash"
git diff-index --quiet HEAD 2>$null
if ($LASTEXITCODE -ne 0) {
    $cmd_version = "$cmd_version.dirty"
} else {
    $exact_tag = (git describe --tags --exact-match 2>$null)
    if ($LASTEXITCODE -eq 0 -and $exact_tag.Trim() -eq "v$version") {
        $cmd_version = $version
    }
}

# Output directory
$outdir = "out\$($env:GOOS)_$($env:GOARCH)"
New-Item -ItemType Directory -Force -Path $outdir | Out-Null

# Put -ldflags in GOFLAGS so Go's own quoted.Split parser, not
# PowerShell's native-command argument builder, handles the embedded space.
$old_go_flags = $env:GOFLAGS
$ldflags = "-X `"github.com/jclark/satpulse/gps/app/cmd.version=$cmd_version`" -X `"github.com/jclark/satpulse/gps/app/cmd.buildDate=$build_date`""
$go_ldflags = "'-ldflags=$ldflags'"
try {
    $env:GOFLAGS = (@($old_go_flags, $go_ldflags) | Where-Object { $_ }) -join ' '
    & go build -tags 'netgo,osusergo' -o $outdir ./cmd/satpulsetool
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    $env:GOFLAGS = $old_go_flags
}

Write-Output "Built satpulsetool in $outdir"
