// Package middleware holds the HTTP middleware stack shared by the v1 and v2
// api-server endpoints: auth, idempotency, rate limit, request logging, panic
// recovery.
//
// Every middleware reads / writes its state on the request context using the
// helper keys declared here so handlers can fetch (tenant, api_key, request_id)
// without knowing which middleware put them there.
package middleware

import "context"

type ctxKey int

const (
	ctxKeyTenantID ctxKey = iota + 1
	ctxKeyAPIKeyID
	ctxKeyAPIKeyPrefix
	ctxKeyRateLimitPerMin
)

// WithTenant stores the active tenant id on ctx.
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ctxKeyTenantID, tenantID)
}

// TenantFromContext returns the tenant id set by the auth middleware, or ""
// if the request did not flow through Auth (e.g. /healthz).
func TenantFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyTenantID).(string)
	return v
}

// WithAPIKey stores the api key id + prefix + rate limit on ctx.
func WithAPIKey(ctx context.Context, id, prefix string, ratePerMin int32) context.Context {
	ctx = context.WithValue(ctx, ctxKeyAPIKeyID, id)
	ctx = context.WithValue(ctx, ctxKeyAPIKeyPrefix, prefix)
	ctx = context.WithValue(ctx, ctxKeyRateLimitPerMin, ratePerMin)
	return ctx
}

// APIKeyIDFromContext returns the api key id resolved by Auth.
func APIKeyIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyAPIKeyID).(string)
	return v
}

// APIKeyPrefixFromContext returns the safe-to-log 8-char prefix.
func APIKeyPrefixFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyAPIKeyPrefix).(string)
	return v
}

// RateLimitFromContext returns the per-minute rate limit configured on the
// API key, or 0 if the request did not flow through Auth.
func RateLimitFromContext(ctx context.Context) int32 {
	v, _ := ctx.Value(ctxKeyRateLimitPerMin).(int32)
	return v
}
