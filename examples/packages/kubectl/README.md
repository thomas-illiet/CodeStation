# kubectl

Installs the official **kubectl v1.33.4** client into the Linux or Windows user profile.

## Verified Internet installation

The module downloads the official binary from `https://dl.k8s.io` for Linux `amd64`/`arm64` or Windows `amd64`, and verifies it against the SHA-256 digest pinned in the signed module before installation.

Internet access to `dl.k8s.io` is required during installation. No kubectl binary is embedded in the CodeStation Packages package or registry.

## Usage

Ensure `~/.local/bin` is present in your `PATH`, then run:

`kubectl version --client`

Uninstalling the package removes only the kubectl binary managed by this module. No Kubernetes configuration or credentials are modified.
