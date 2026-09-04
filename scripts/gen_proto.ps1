$ErrorActionPreference = "Continue"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$RootDir = Split-Path -Parent $ScriptDir
$ProtoDir = Join-Path $RootDir "proto"
$GoOutDir = Join-Path $RootDir "go-services\shared\pb"
$PyOutDir = Join-Path $RootDir "python-ml\shared\pb"

Write-Host "=== Aegis Protobuf Generator (PowerShell) ==="
Write-Host "Proto dir: $ProtoDir"
Write-Host "Go output: $GoOutDir"
Write-Host "Python output: $PyOutDir"

New-Item -ItemType Directory -Force -Path $GoOutDir | Out-Null
New-Item -ItemType Directory -Force -Path $PyOutDir | Out-Null

$protoFiles = Get-ChildItem -Path $ProtoDir -Filter *.proto | Select-Object -ExpandProperty FullName

if (Get-Command protoc -ErrorAction SilentlyContinue) {
    Write-Host "Generating Go stubs..."
    & protoc "-I=$ProtoDir" "--go_out=$RootDir\go-services" "--go_opt=module=github.com/aegis/go-services" "--go-grpc_out=$RootDir\go-services" "--go-grpc_opt=module=github.com/aegis/go-services" $protoFiles
} else {
    Write-Host "Notice: protoc not found in PATH."
}

try {
    & python -m grpc_tools.protoc --version | Out-Null
    Write-Host "Generating Python stubs..."
    & python -m grpc_tools.protoc "-I=$ProtoDir" "--python_out=$PyOutDir" "--grpc_python_out=$PyOutDir" $protoFiles
    New-Item -ItemType File -Force -Path (Join-Path $PyOutDir "__init__.py") | Out-Null
} catch {
    Write-Host "Notice: python grpc_tools not installed, skipping Python stub generation."
}

Write-Host "Proto generation script finished."
