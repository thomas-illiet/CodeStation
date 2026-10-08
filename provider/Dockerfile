FROM golang:1.24-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY provider/go.mod provider/go.sum ./
RUN go mod download
COPY provider/ ./
RUN set -eu; \
    executable=terraform-provider-packages_v1.0.0; \
    if [ "$TARGETOS" = windows ]; then executable="$executable.exe"; fi; \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o "/out/$executable" .

FROM scratch
COPY --from=build /out/ /
