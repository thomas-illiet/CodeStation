terraform {
  required_version = ">= 1.8.0"
  required_providers { codestation = { source = "codestation.internal/codestation/packages", version = "1.0.0" } }
}
locals { is_windows = fileexists("C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe") }
resource "codestation_binary" "yq" {
  version     = "4.54.1"
  destination = local.is_windows ? "~/.local/bin/yq.exe" : "~/.local/bin/yq"
  urls = {
    linux_amd64   = "https://github.com/mikefarah/yq/releases/download/v4.54.1/yq_linux_amd64"
    linux_arm64   = "https://github.com/mikefarah/yq/releases/download/v4.54.1/yq_linux_arm64"
    windows_amd64 = "https://github.com/mikefarah/yq/releases/download/v4.54.1/yq_windows_amd64.exe"
  }
  sha256s = {
    linux_amd64   = "8e34fc298390875de416e6a4afcb8cabeceb25d9aa8506c1a2f9353cf702ea5f"
    linux_arm64   = "189088da0c6429ec5178dfaab1a114805f6cab0b61b165ab236efedf1d57a71b"
    windows_amd64 = "b645f47ebb3a0d2fbab52998550bbd0a1706f23b5ca5f5b48e638c580bb70928"
  }
}
