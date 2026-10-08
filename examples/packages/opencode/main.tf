terraform {
  required_version = ">= 1.8.0"
  required_providers { codestation = { source = "codestation.internal/codestation/packages", version = "1.0.0" } }
}
locals { is_windows = fileexists("C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe") }
resource "codestation_archive" "opencode" {
  version     = "1.18.33"
  destination = local.is_windows ? "~/.local/bin/opencode.exe" : "~/.local/bin/opencode"
  urls = {
    linux_amd64   = "https://github.com/anomalyco/opencode/releases/download/v1.18.33/opencode-linux-x64.tar.gz"
    linux_arm64   = "https://github.com/anomalyco/opencode/releases/download/v1.18.33/opencode-linux-arm64.tar.gz"
    windows_amd64 = "https://github.com/anomalyco/opencode/releases/download/v1.18.33/opencode-windows-x64.zip"
  }
  sha256s = {
    linux_amd64   = "e546123213ae47909a4268692aa4b94950d011afe9cac9938753a2194f1c16d5"
    linux_arm64   = "c63486624621924bf43be5c01abd252885661a734814224f6d70188a33aea858"
    windows_amd64 = "cc827fda2e32502373c5de25a4b78471766b99881d12a441ae5b9fcff39c5780"
  }
  members = { linux_amd64 = "opencode", linux_arm64 = "opencode", windows_amd64 = "opencode.exe" }
}
