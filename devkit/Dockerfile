FROM golang:1.24-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY devkit/go.mod devkit/go.sum ./
RUN go mod download
COPY devkit .
COPY bin /toolchain/bin
COPY providers/codestation.internal/codestation/packages/1.0.0 /toolchain/providers
RUN tofu_arch=${TARGETARCH} && if [ "$TARGETARCH" = amd64 ]; then tofu_arch=x64; fi && \
    cp "/toolchain/bin/${TARGETOS}-${tofu_arch}/tofu" internal/toolchain/assets/tofu && \
    cp "/toolchain/providers/${TARGETOS}_${TARGETARCH}/terraform-provider-packages_v1.0.0" internal/toolchain/assets/provider && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/codestation ./cmd/codestation

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/codestation /usr/local/bin/codestation
ENTRYPOINT ["/usr/local/bin/codestation"]
