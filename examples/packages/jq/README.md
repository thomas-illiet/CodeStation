# jq

Installs **jq 1.8.2**, the lightweight command-line JSON processor, into the user profile.

The signed module downloads the official standalone binary for Linux `amd64`/`arm64` or Windows `amd64` from the `jqlang/jq` GitHub release and verifies its pinned SHA-256 digest before installation.

Internet access to GitHub release assets is required. No jq binary is stored in CodeStation Packages.

## Usage

`printf '{"status":"ok"}' | jq .status`

Uninstalling removes only the binary managed by CodeStation Packages.
