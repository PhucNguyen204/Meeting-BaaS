# Bot-worker container — Go binary + Xvfb + PulseAudio + Playwright deps.
#
# Heavily inspired by the root Dockerfile. Phase 1 only needs the runtime
# environment to launch Chromium via playwright-go and verify Meet flows;
# FFmpeg/EFS bits are left in place so Phase 2 (recorder) can reuse this
# image without changes.

FROM golang:1.24-bookworm AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-X github.com/yourorg/meet-bot-go/internal/pkg/version.Version=${VERSION} \
              -X github.com/yourorg/meet-bot-go/internal/pkg/version.Commit=${COMMIT} \
              -X github.com/yourorg/meet-bot-go/internal/pkg/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /out/bot-worker ./cmd/bot-worker

# ----------------------------------------------------------------------
FROM ubuntu:24.04 AS runtime

ENV DEBIAN_FRONTEND=noninteractive \
    DISPLAY=:99 \
    PULSE_RUNTIME_PATH=/tmp/pulse \
    XDG_RUNTIME_DIR=/tmp/pulse \
    LOG_LEVEL=info \
    LOG_DIR=/var/log/bot

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl unzip \
        # Browser deps (mirror root Dockerfile)
        wget libnss3 libatk-bridge2.0-0 libdrm2 libxkbcommon0 \
        libxcomposite1 libxdamage1 libxrandr2 libgbm1 libxss1 libxshmfence1 \
        # Virtual display + audio
        xvfb x11vnc x11-utils pulseaudio pulseaudio-utils unclutter \
        # Media processing
        ffmpeg \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/bot-worker /usr/local/bin/bot-worker
COPY deployments/docker/start-bot-worker.sh /start.sh
RUN chmod +x /start.sh

# Playwright driver download is performed at first run by playwright-go itself
# (it caches into /root/.cache/ms-playwright-go). For prod we recommend baking
# the driver in via a separate stage; for Phase 1 the runtime download is fine.

EXPOSE 8080 5900
ENTRYPOINT ["/start.sh"]
