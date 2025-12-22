package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/propagation"
)

const (
	// ZookieHeader is the header name for Zookie propagation
	ZookieHeader = "zenith-zookie"
)

// ZookiePropagator is a custom propagator that carries Zookie through trace context
type ZookiePropagator struct{}

// Inject injects the Zookie into the carrier
func (p *ZookiePropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	if zookie := ctx.Value("zenith.zookie"); zookie != nil {
		if zookieInt, ok := zookie.(int64); ok {
			carrier.Set(ZookieHeader, fmt.Sprintf("%d", zookieInt))
		}
	}
}

// Extract extracts the Zookie from the carrier
func (p *ZookiePropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	zookieStr := carrier.Get(ZookieHeader)
	if zookieStr != "" {
		var zookie int64
		if _, err := fmt.Sscanf(zookieStr, "%d", &zookie); err == nil {
			ctx = context.WithValue(ctx, "zenith.zookie", zookie)
		}
	}
	return ctx
}

// Fields returns the fields that this propagator uses
func (p *ZookiePropagator) Fields() []string {
	return []string{ZookieHeader}
}

