# Claude Code

Installs **Claude Code v2.1.211** in the Linux or Windows user profile.

## Installation

On Linux, the module uses Anthropic's official native installer. On Windows, it uses Anthropic's supported npm package and requires Node.js, npm, and Git for Windows. Both paths pin version 2.1.211.

Internet access to `claude.ai` and Anthropic's download service is required during installation. The official installer performs the release download and integrity verification.

## Usage

Ensure `~/.local/bin` on Linux or `~/.local` on Windows is in `PATH`, then run:

`claude`

Claude Code requires an eligible Claude or Anthropic Console account. Uninstalling this package removes the CodeStation Packages-managed launcher and pinned executable while preserving settings and sessions stored in `~/.claude`.
