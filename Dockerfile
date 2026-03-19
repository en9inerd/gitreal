# ---------- Build ----------
# Pin builder to the build machine's native platform so Go cross-compiles
# natively instead of running under QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.26.1-alpine AS builder

RUN apk update && apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
      -gcflags="all=-l -B" \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /gitreal \
      ./cmd/gitreal

# ---------- Runtime ----------
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /gitreal /app/gitreal

USER app

EXPOSE 8080

HEALTHCHECK CMD wget -q -O /dev/null http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/gitreal"]
