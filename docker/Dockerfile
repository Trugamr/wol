FROM golang:1.26.4-alpine3.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o wol

#------------------#

FROM gcr.io/distroless/static-debian12 AS release

COPY --from=builder /app/wol /usr/local/bin/wol

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/wol"]