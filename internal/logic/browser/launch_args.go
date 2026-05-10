package browser

import (
	"fmt"
	"os"
)

// Default Chromium command-line arguments for the bot.
//
// Port reference: src/browser/browser.ts:32-86 (the entire `args:` array
// in launchPersistentContext). Keep this list in sync — these flags were
// hard-won and stripping any of them tends to break audio capture or
// WebRTC on Meet.
//
// The list is split into logical groups for readability.
//
// TODO(user): keep this in lock-step with the upstream TS file when
// updating playwright-go or Chromium.
func defaultArgs(opts LaunchOptions) []string {
	width, height := resolutionPixels(opts.Resolution)

	args := []string{
		// --- Window size & position (must match Xvfb) ---
		fmt.Sprintf("--window-size=%d,%d", width, height),
		"--window-position=0,0",

		// --- Sandbox (containers don't have user namespaces) ---
		"--no-sandbox",
		"--disable-setuid-sandbox",
		"--lang=en-US",
		"--accept-lang=en-US,en",

		// --- Audio (PulseAudio in container) ---
		"--use-pulseaudio",
		"--enable-audio-service-sandbox=false",
		"--audio-buffer-size=2048",
		"--disable-features=AudioServiceSandbox",
		"--autoplay-policy=no-user-gesture-required",

		// --- WebRTC tweaks ---
		"--disable-rtc-smoothness-algorithm",
		"--disable-webrtc-hw-decoding",
		"--disable-webrtc-hw-encoding",
		"--enable-webrtc-capture-audio",
		"--force-webrtc-ip-handling-policy=default",

		// --- Performance ---
		"--disable-blink-features=AutomationControlled",
		"--disable-background-timer-throttling",
		"--enable-features=SharedArrayBuffer",
		"--memory-pressure-off",
		"--max_old_space_size=4096",
		"--disable-background-networking",
		"--disable-features=TranslateUI",
		"--disable-features=AutofillServerCommunication",
		"--disable-component-extensions-with-background-pages",
		"--disable-default-apps",
		"--renderer-process-limit=4",
		"--disable-ipc-flooding-protection",
		"--aggressive-cache-discard",
		"--disable-features=MediaRouter",

		// --- Cert/security ---
		"--ignore-certificate-errors",
		"--allow-insecure-localhost",
		"--disable-blink-features=TrustedDOMTypes",
		"--disable-features=TrustedScriptTypes",
		"--disable-features=TrustedHTML",

		// --- Audio debug logging ---
		"--enable-logging=stderr",
		"--log-level=1",
		"--vmodule=*audio*=3",
	}
	return args
}

// ResolveChromePath returns the executable path, honouring opts then
// $CHROME_PATH then a default of /usr/bin/google-chrome.
func ResolveChromePath(opts LaunchOptions) string {
	if opts.ChromePath != "" {
		return opts.ChromePath
	}
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
	}
	return "/usr/bin/google-chrome"
}

// Viewport dimensions corresponding to the requested resolution. The window
// is taller than the viewport to leave room for browser chrome (~140px),
// matching the Xvfb settings in deployments/docker/start-bot-worker.sh.
//
// Port reference: src/browser/browser.ts:9-18 + Dockerfile resolution block.
func resolutionPixels(resolution string) (int, int) {
	if resolution == "1080" {
		return 1920, 1220
	}
	return 1280, 860
}

// ViewportPixels returns the inner viewport (without browser chrome) for
// the given resolution.
func ViewportPixels(resolution string) (int, int) {
	if resolution == "1080" {
		return 1920, 1080
	}
	return 1280, 720
}
