# Renders the HTML sources in docs\src to PDF with headless Microsoft Edge.
#   docs\bareboot-documentacion.pdf      technical documentation
#   docs\bareboot-guia-demostracion.pdf  demonstration guide
$ErrorActionPreference = 'Stop'

$edge = 'C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe'
if (-not (Test-Path $edge)) { throw "Microsoft Edge not found at $edge" }

$docs    = $PSScriptRoot
$profile = Join-Path $env:TEMP 'bareboot-docs-edge'   # separate profile: never touches the user's browser

$jobs = @{
    'documentacion.html'     = 'bareboot-documentacion.pdf'
    'guia-demostracion.html' = 'bareboot-guia-demostracion.pdf'
}
foreach ($src in $jobs.Keys) {
    $in  = Join-Path $docs "src\$src"
    $out = Join-Path $docs $jobs[$src]
    Start-Process $edge -Wait -ArgumentList @(
        '--headless=new', '--disable-gpu', '--no-first-run', "--user-data-dir=$profile",
        '--no-pdf-header-footer', '--virtual-time-budget=5000',
        "--print-to-pdf=$out", ('file:///' + $in.Replace('\', '/')))
    Write-Host "OK: $out"
}
