FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/go_substack_scraper .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget tini \
 && addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=builder /out/go_substack_scraper /app/go_substack_scraper
COPY --from=builder /app/service.yaml /app/service.yaml

USER app

EXPOSE 8236

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${PORT:-8236}/health >/dev/null || exit 1

ENTRYPOINT ["/sbin/tini","--"]
CMD ["/app/go_substack_scraper"]
