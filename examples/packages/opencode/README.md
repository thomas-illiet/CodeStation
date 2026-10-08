# OpenCode

Installs the standalone **OpenCode v1.18.33** terminal coding agent into the Linux or Windows user profile.

## Installation

The module downloads the official Linux or Windows release archive from GitHub, automatically selects `x64` or `arm64`, and verifies its pinned SHA-256 digest before installation.

Internet access to `github.com` is required during installation. The OpenCode binary is not embedded in the CodeStation Packages package or registry.

## Usage

Ensure `~/.local/bin` is present in your `PATH`, then run:

`opencode`

OpenCode stores its own configuration and session data separately from the executable. Uninstalling this package removes only the `opencode` binary managed by CodeStation Packages; it preserves the user's OpenCode configuration and data.
