FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

FROM alpine:3.20
# hadolint DL3018: pin versions for reproducible image builds (bump when Alpine updates packages)
RUN apk add --no-cache ca-certificates=20250911-r0 tzdata=2026a-r0
COPY --from=build /out/bot /bot
USER nobody
ENTRYPOINT ["/bot"]
# Process liveness (bot has no HTTP listen port); satisfies CKV_DOCKER_2 / Trivy AVD-DS-0026
HEALTHCHECK --interval=60s --timeout=5s --start-period=30s --retries=3 \
  CMD /bin/sh -c 'test -n "$(pidof bot)"'
