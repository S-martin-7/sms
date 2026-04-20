package httpx

import "context"

type tenantIDKey struct{}
type adminIDKey struct{}
type adminRoleKey struct{}

// SetTenantID stores the authenticated tenant's id into ctx.
func SetTenantID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, tenantIDKey{}, id)
}

// TenantIDFrom returns the tenant id stored by auth middleware, or 0.
func TenantIDFrom(ctx context.Context) int64 {
	if v, ok := ctx.Value(tenantIDKey{}).(int64); ok {
		return v
	}
	return 0
}

// SetAdmin stores the authenticated admin's (id, role) into ctx.
func SetAdmin(ctx context.Context, id int64, role string) context.Context {
	ctx = context.WithValue(ctx, adminIDKey{}, id)
	return context.WithValue(ctx, adminRoleKey{}, role)
}

// AdminIDFrom returns the admin id stored by JWT middleware, or 0.
func AdminIDFrom(ctx context.Context) int64 {
	if v, ok := ctx.Value(adminIDKey{}).(int64); ok {
		return v
	}
	return 0
}

// AdminRoleFrom returns the admin role stored by JWT middleware, or "".
func AdminRoleFrom(ctx context.Context) string {
	if v, ok := ctx.Value(adminRoleKey{}).(string); ok {
		return v
	}
	return ""
}
