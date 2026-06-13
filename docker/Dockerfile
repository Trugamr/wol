# Build on the runner's native arch and cross-compile to the target arch, so
# multi-arch builds don't emulate the whole Go toolchain under QEMU.
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine3.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
# Version info, injected via ldflags. Defaults keep a plain `docker build` working.
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w \
      -X github.com/trugamr/wol/cmd.version=${VERSION} \
      -X github.com/trugamr/wol/cmd.commit=${COMMIT} \
      -X github.com/trugamr/wol/cmd.date=${DATE}" -o wol

#------------------#

FROM gcr.io/distroless/static-debian12 AS release

COPY --from=builder /app/wol /usr/local/bin/wol

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/wol"]