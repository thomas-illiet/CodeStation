# K9s

Installs **K9s v0.51.0**, a terminal UI for managing Kubernetes clusters, into the user profile.

## Verified Internet installation

The module downloads the official archive for Linux `amd64`/`arm64` or Windows `amd64` from the `derailed/k9s` GitHub release and verifies it against the SHA-256 digest pinned in this signed module.

Internet access to `github.com` and its release asset hosts is required during installation. No K9s binary is embedded in the CodeStation Packages registry.

## Usage

Ensure `~/.local/bin` is in your `PATH`, configure access to a Kubernetes cluster, then run:

`k9s`

Uninstalling removes only the managed executable and temporary download cache. Existing Kubernetes credentials and K9s configuration are preserved.
