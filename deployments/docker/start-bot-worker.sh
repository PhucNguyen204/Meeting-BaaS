#!/usr/bin/env bash
# Container entrypoint: bring up Xvfb + PulseAudio + (optional) VNC,
# then exec the Go bot-worker. Mirrors the bash script in the root
# Dockerfile but tailored for the Go binary.
set -euo pipefail

mkdir -p "${PULSE_RUNTIME_PATH:-/tmp/pulse}"
mkdir -p "${LOG_DIR:-/var/log/bot}"

# --- Resolution ---------------------------------------------------------
RESOLUTION="${RESOLUTION:-720}"
if [[ "$RESOLUTION" == "1080" ]]; then
    X11_WIDTH=1920
    X11_HEIGHT=1220
else
    X11_WIDTH=1280
    X11_HEIGHT=860
fi
echo "[start] Using resolution ${X11_WIDTH}x${X11_HEIGHT}"

# --- Xvfb ---------------------------------------------------------------
Xvfb "$DISPLAY" -screen 0 "${X11_WIDTH}x${X11_HEIGHT}x24" \
    -ac +extension GLX +render -noreset -nocursor -nolisten tcp &
XVFB_PID=$!
sleep 2
unclutter -display "$DISPLAY" -idle 0 -root &

# --- VNC (optional, debug only) ----------------------------------------
if [[ "${ENABLE_VNC:-false}" == "true" ]]; then
    x11vnc -display "$DISPLAY" -forever -passwd "${VNC_PASSWORD:-debug}" \
        -listen 0.0.0.0 -rfbport 5900 -shared -nocursor -bg \
        -o /tmp/x11vnc.log
fi

# --- PulseAudio ---------------------------------------------------------
pulseaudio --start --log-target=stderr --log-level=notice
sleep 2
if ! pactl info >/dev/null 2>&1; then
    echo "[start] PulseAudio failed, retrying" >&2
    pulseaudio --kill || true
    sleep 1
    pulseaudio --start --log-target=stderr --log-level=notice
    sleep 2
fi

pactl load-module module-null-sink sink_name=virtual_speaker \
    sink_properties=device.description=Virtual_Speaker,device.class=sound
pactl load-module module-virtual-source source_name=virtual_mic
pactl set-default-sink virtual_speaker

if ! pactl list sources short | grep -q "virtual_speaker.monitor"; then
    echo "[start] virtual_speaker.monitor not found - audio setup failed" >&2
    exit 1
fi

# --- bot-worker ---------------------------------------------------------
echo "[start] Launching bot-worker"
exec /usr/local/bin/bot-worker "$@"
