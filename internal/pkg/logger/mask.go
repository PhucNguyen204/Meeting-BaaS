package logger

import (
	"strings"
)

// Sensitive keys borrowed from the TS implementation. Any key matching
// (case-insensitive, substring) one of these will have its value replaced
// with maskValue when passed through MaskSecrets.
//
// Port reference: src/main.ts:217-224 (logParams masking block).
var sensitiveKeySubstrings = []string{
	"token",
	"api_key",
	"apikey",
	"secret",
	"password",
	"pwd",
	"webhook",
}

const maskValue = "***MASKED***"

// MaskSecrets returns a shallow copy of m with values for sensitive keys
// replaced by maskValue. Maps and slices nested inside are NOT recursed
// into â€” the caller should only pass flat parameter maps (the TS code does
// the same).
//
// Use this whenever a config blob is logged at startup so credentials never
// land in the log file.
func MaskSecrets(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if isSensitiveKey(k) {
			out[k] = maskValue
			continue
		}
		out[k] = v
	}
	return out
}

// isSensitiveKey reports whether the key should be masked.
//
// Match is case-insensitive substring against sensitiveKeySubstrings.
func isSensitiveKey(key string) bool {
	lk := strings.ToLower(key)
	for _, needle := range sensitiveKeySubstrings {
		if strings.Contains(lk, needle) {
			return true
		}
	}
	return false
}
