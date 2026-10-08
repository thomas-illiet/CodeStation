terraform {
  required_version = ">= 1.8.0"
  required_providers { codestation = { source = "codestation.internal/codestation/packages", version = "1.0.0" } }
}
locals { is_windows = fileexists("C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe") }
resource "codestation_archive" "k9s" {
  version     = "0.51.0"
  destination = local.is_windows ? "~/.local/bin/k9s.exe" : "~/.local/bin/k9s"
  urls = {
    linux_amd64   = "https://github.com/derailed/k9s/releases/download/v0.51.0/k9s_Linux_amd64.tar.gz"
    linux_arm64   = "https://github.com/derailed/k9s/releases/download/v0.51.0/k9s_Linux_arm64.tar.gz"
    windows_amd64 = "https://github.com/derailed/k9s/releases/download/v0.51.0/k9s_Windows_amd64.zip"
  }
  sha256s = {
    linux_amd64   = "c3752ad51a5a4015a113819c4eeb6e55a4d0e4b8e652494797532f6fc8161dd7"
    linux_arm64   = "3ee05c82e5f9198928a4e86133608ba6a2c10a2244d6a7789e820f78319d640c"
    windows_amd64 = "bba299f1877913979831543a008f2e104995d2ea4ac8f23d7f49c0444857d973"
  }
  members = { linux_amd64 = "k9s", linux_arm64 = "k9s", windows_amd64 = "k9s.exe" }
}
