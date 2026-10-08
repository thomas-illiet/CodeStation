terraform {
  required_version = ">= 1.8.0"
  required_providers { codestation = { source = "codestation.internal/codestation/packages", version = "1.0.0" } }
}
resource "codestation_file" "large_install" {
  version     = "1.0.0"
  destination = "~/.local/share/codestation-large-install/components.txt"
  content     = join("", [for step in range(1, 21) : format("component-%02d installed\n", step)])
}
