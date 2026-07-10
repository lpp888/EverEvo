# Standalone Rust engine build script
Push-Location $PSScriptRoot\..\engine
cargo build --release
Pop-Location
Write-Host "Engine built: engine/target/release/engine.lib"
