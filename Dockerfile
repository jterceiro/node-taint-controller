# Stage 1 – Build
FROM golang:1.26-alpine AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/    cmd/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /manager ./cmd/

# Stage 2 – Minimal distroless runtime image
FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.source="https://github.com/jterceiro/node-taint-controller"
LABEL org.opencontainers.image.licenses="MIT"

COPY --from=builder /manager /manager

USER nonroot:nonroot
ENTRYPOINT ["/manager"]
