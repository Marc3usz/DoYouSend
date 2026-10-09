package iam

import (
	"net/http"
	"testing"
)

func TestRouteAccess(t *testing.T) {
	tests := []struct {
		method, path string
		want         Access
	}{
		{http.MethodGet, "/healthz", AccessPublic},
		{http.MethodPost, "/api/auth/login", AccessPublic},
		{http.MethodGet, "/api/auth/login", AccessUser},
		{http.MethodPost, "/providers/sms/dlr", AccessPublic},
		{http.MethodGet, "/providers/sms/dlr/token", AccessPublic},

		{http.MethodGet, "/api/auth/me", AccessUser},
		{http.MethodPost, "/api/auth/logout", AccessUser},
		{http.MethodPost, "/api/messages/preview", AccessUser},
		{http.MethodPost, "/api/batches", AccessUser},
		{http.MethodGet, "/api/groups", AccessUser},
		{http.MethodPost, "/api/groups/resolve", AccessUser},
		{http.MethodGet, "/api/something-new", AccessUser},

		{http.MethodPost, "/api/groups", AccessAdmin},
		{http.MethodGet, "/api/groups/1", AccessAdmin},
		{http.MethodPut, "/api/groups/1/members", AccessAdmin},
		{http.MethodGet, "/api/recipients", AccessAdmin},
		{http.MethodPost, "/api/recipients/import", AccessAdmin},
		{http.MethodGet, "/api/users", AccessAdmin},
		{http.MethodPatch, "/api/users/1", AccessAdmin},
		{http.MethodGet, "/api/audit", AccessAdmin},
		{http.MethodGet, "/api/stats/sms", AccessAdmin},
		{http.MethodGet, "/api/admin/config", AccessAdmin},

		// Segment matching, not a bare string prefix.
		{http.MethodGet, "/api/usersfoo", AccessUser},
		{http.MethodGet, "/api/groupsx", AccessUser},
	}
	for _, tt := range tests {
		if got := RouteAccess(tt.method, tt.path); got != tt.want {
			t.Errorf("RouteAccess(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}
