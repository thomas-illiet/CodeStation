terraform {
  required_version = ">= 1.8.0"
  required_providers { codestation = { source = "codestation.internal/codestation/packages", version = "1.0.0" } }
}
resource "codestation_npm_package" "claude_code" {
  package = "@anthropic-ai/claude-code"
  version = "2.1.211"
  prefix  = "~/.local"
}
