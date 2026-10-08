ARG CODE_SERVER_VERSION=4.140.0

FROM golang:1.24-bookworm AS provider-builder
WORKDIR /build/provider
COPY provider/go.mod provider/go.sum ./
RUN go mod download
COPY provider/ ./
RUN set -eu; \
    for target in linux_amd64 linux_arm64 windows_amd64; do \
      os=${target%_*}; arch=${target#*_}; executable=terraform-provider-packages_v1.0.0; \
      if [ "$os" = windows ]; then executable="$executable.exe"; fi; \
      directory="/out/providers/codestation.internal/codestation/packages/1.0.0/$target"; \
      mkdir -p "$directory"; \
      CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$directory/$executable" .; \
    done

FROM node:22-bookworm AS extension-builder
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /build/extension
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY tsconfig.json ./
COPY src/ src/
COPY docs/ docs/
COPY media/ media/
COPY bin/ bin/
COPY keys/ keys/
COPY --from=provider-builder /out/providers/ providers/
COPY demo/keys/demo-signing-public.pem keys/demo-2026.pem
RUN set -eu; \
    printf '%s  %s\n' a325c8c2f6834575e440b03c2ba67f94256072754ac7787fea718be6f01fef6a bin/linux-x64/tofu | sha256sum -c -; \
    printf '%s  %s\n' d5c690023baebe8bf2cfb2062e292186f4797a173db22c973bd9de45cb68a62f bin/linux-arm64/tofu | sha256sum -c -; \
    printf '%s  %s\n' 554ec8acba937e516c5b6ba93ee46172420f6365e8dfe41e70218157cd4a802d bin/windows-x64/tofu.exe | sha256sum -c -; \
    mkdir -p /out; \
    npm run package; \
    cp ./*.vsix /out/codestation.vsix

FROM ghcr.io/coder/code-server:${CODE_SERVER_VERSION}
USER root
COPY --from=extension-builder /usr/local/bin/node /usr/local/bin/node
COPY --from=extension-builder /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm
COPY --from=extension-builder /out/codestation.vsix /opt/codestation/codestation.vsix
COPY demo/code-server-entrypoint.sh /usr/local/bin/codestation-demo-entrypoint
COPY demo/workspace/ /opt/codestation/workspace/
RUN apt-get update && apt-get install -y --no-install-recommends netcat-openbsd && rm -rf /var/lib/apt/lists/* && \
    ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && \
    mkdir -p /workspace && cp -R /opt/codestation/workspace/. /workspace/ && \
    chown -R coder:coder /workspace && chmod 0755 /usr/local/bin/codestation-demo-entrypoint
USER coder
ENTRYPOINT ["/usr/local/bin/codestation-demo-entrypoint"]
CMD ["code-server", "--bind-addr", "0.0.0.0:8080", "--auth", "none", "/workspace"]
