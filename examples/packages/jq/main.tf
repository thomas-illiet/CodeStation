terraform {
  required_version = ">= 1.8.0"
  required_providers { codestation = { source = "codestation.internal/codestation/packages", version = "1.0.0" } }
}
locals { is_windows = fileexists("C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe") }
resource "codestation_binary" "jq" {
  version     = "1.8.2"
  destination = local.is_windows ? "~/.local/bin/jq.exe" : "~/.local/bin/jq"
  urls = {
    linux_amd64   = "https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-linux-amd64"
    linux_arm64   = "https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-linux-arm64"
    windows_amd64 = "https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-windows-amd64.exe"
  }
  sha256s = {
    linux_amd64   = "b1c22172dd303f3be49e935aa56aa48a8b7a46e0bc838b4997d3bb451495870f"
    linux_arm64   = "8b85c817833814ddca00a144c33705546355afccf0cf39b188f3cdb48b852309"
    windows_amd64 = "a6fc67fedaf9128a3309a1e2ebb8b986aeccf70122ee46d2cb4849e423f0c627"
  }
}
