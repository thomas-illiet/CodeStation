FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/codestation-registry ./cmd/codestation-registry
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/codestation-registry /codestation-registry
# Docker initializes a new named volume with the ownership of the directory in
# the image. Keep /data writable by distroless' nonroot user (UID/GID 65532).
COPY --from=build --chown=65532:65532 /out/data /data
EXPOSE 8080
ENTRYPOINT ["/codestation-registry"]
