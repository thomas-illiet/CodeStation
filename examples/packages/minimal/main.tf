terraform {
  required_version = ">= 1.8.0"
  required_providers { codestation = { source = "codestation.internal/codestation/packages", version = "1.0.0" } }
}
resource "codestation_file" "hello" {
  version     = "1.0.0"
  destination = "~/.local/share/codestation-packages-demo/message.txt"
  content     = "hello from CodeStation Packages\n"
}
