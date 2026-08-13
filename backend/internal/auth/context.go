package auth

import "context"

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal SessionPrincipal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (SessionPrincipal, bool) {
	value := ctx.Value(principalContextKey{})
	if value == nil {
		return SessionPrincipal{}, false
	}
	principal, ok := value.(SessionPrincipal)
	return principal, ok
}
