# ==========================================
# STAGE 1: Builder
# ==========================================
FROM golang:1.27-alpine AS builder

WORKDIR /build

COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o caduceus ./cmd/caduceus

# ==========================================
# STAGE 2: Minimal Runtime Sandbox
# ==========================================
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

RUN adduser -D -u 1000 caduceus
USER caduceus
WORKDIR /home/caduceus

COPY --from=builder /build/caduceus /usr/local/bin/caduceus

VOLUME ["/home/caduceus/.config/caduceus"]

EXPOSE 8443

ENTRYPOINT [ "caduceus" ]
