terraform {
  required_version = ">= 1.8.0"
  required_providers { codestation = { source = "codestation.internal/codestation/packages", version = "1.0.0" } }
}
locals { is_windows = fileexists("C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe") }
resource "codestation_binary" "kubectl" {
  version     = "1.33.4"
  destination = local.is_windows ? "~/.local/bin/kubectl.exe" : "~/.local/bin/kubectl"
  urls = {
    linux_amd64   = "https://dl.k8s.io/release/v1.33.4/bin/linux/amd64/kubectl"
    linux_arm64   = "https://dl.k8s.io/release/v1.33.4/bin/linux/arm64/kubectl"
    windows_amd64 = "https://dl.k8s.io/release/v1.33.4/bin/windows/amd64/kubectl.exe"
  }
  sha256s = {
    linux_amd64   = "c2ba72c115d524b72aaee9aab8df8b876e1596889d2f3f27d68405262ce86ca1"
    linux_arm64   = "76cd7a2aa59571519b68c3943521404cbce55dafb7d8866f8d0ea2995b396eef"
    windows_amd64 = "15487c2a017af8ef8a5fbc2390af78b90f98d2909f23eeb684c64a8af3f7c4eb"
  }
}
