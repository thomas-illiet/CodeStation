# CodeStation Packages dev kit

The dev kit is a standalone Go binary. Release builds embed OpenTofu 1.13.1 and
the CodeStation provider for exactly one supported target: Linux amd64, Linux
arm64, or Windows amd64. An OCI image exposes the same interface.

```text
codestation package init DIRECTORY [options]
codestation package [--chdir DIRECTORY] plan [OPEN_TOFU_ARGUMENTS...]
codestation package [--chdir DIRECTORY] apply [OPEN_TOFU_ARGUMENTS...]
codestation package [--chdir DIRECTORY] destroy [OPEN_TOFU_ARGUMENTS...]
codestation package upload [DIRECTORY] --registry URL [--token TOKEN]
codestation package delete NAMESPACE/NAME@VERSION --registry URL [--token TOKEN] [--yes]
```

`plan`, `apply`, and `destroy` are transparent OpenTofu pass-throughs. The dev
kit only selects the embedded executable and provider mirror, changes into the
requested directory, and connects the original standard streams.

Build the local image with `make devkit-image`, or build all release binaries
and their `SHA256SUMS` file with `make devkit-release`.
