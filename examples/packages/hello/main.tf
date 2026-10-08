terraform {
  required_version = ">= 1.8.0"
  required_providers { codestation = { source = "codestation.internal/codestation/packages", version = "1.0.0" } }
}
locals { is_windows = fileexists("C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe") }
resource "codestation_file" "hello" {
  version     = "1.0.0"
  destination = local.is_windows ? "~/.local/bin/codestation-hello.cmd" : "~/.local/bin/codestation-hello"
  content     = local.is_windows ? "@echo Hello from CodeStation Packages!\r\n" : "#!/usr/bin/env sh\nprintf 'Hello from CodeStation Packages!\\n'\n"
}
output "installed_command" { value = local.is_windows ? "~\\.local\\bin\\codestation-hello.cmd" : "~/.local/bin/codestation-hello" }
