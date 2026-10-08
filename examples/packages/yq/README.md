# yq

Installs **yq v4.54.1**, a command-line processor for YAML, JSON, XML, CSV, TOML and properties, into the Linux or Windows user profile.

The signed module downloads the official standalone binary for Linux `amd64`/`arm64` or Windows `amd64` from the `mikefarah/yq` GitHub release and verifies its pinned SHA-256 digest before installation.

Internet access to GitHub release assets is required. No yq binary is stored in CodeStation Packages.

## Usage

`yq '.metadata.name' deployment.yaml`

Uninstalling removes only the binary managed by CodeStation Packages.
