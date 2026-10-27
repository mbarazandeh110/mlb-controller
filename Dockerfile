# Stage 1: Builder
FROM golang:1.24.7-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# بهینه‌سازی دقیق‌تر کامپایل: تگ netgo برای جلوگیری از وابستگی به glibc در رزولوشن DNS
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -tags netgo \
    -ldflags="-s -w -extldflags '-static'" \
    -o mlb-controller ./cmd/mlb-controller

# Stage 2: Final Minimal Image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# کپی باینری از مرحله قبل
COPY --from=builder /app/mlb-controller /mlb-controller

# کپی فایل‌های کانفیگ با دسترسی nonroot (UID 65532 در Distroless)
COPY --from=builder --chown=65532:65532 /app/configs /configs

ENV CONFIG_PATH=/configs/config.yaml

EXPOSE 9090

ENTRYPOINT ["/mlb-controller"]

