package iam

import (
	"net/http"
	"strings"
)

// Access is who may call a route.
type Access int

const (
	// AccessUser requires any logged-in user. It is the default: a route
	// nobody thought about is never public by accident.
	AccessUser Access = iota
	// AccessPublic needs no session.
	AccessPublic
	// AccessAdmin requires the admin role.
	AccessAdmin
)

func (a Access) String() string {
	switch a {
	case AccessPublic:
		return "public"
	case AccessAdmin:
		return "admin"
	default:
		return "user"
	}
}

// rule matches a method (empty = any) and a path prefix, or an exact path when
// exact is set. With suffix, the path must be path + one segment (an ID) +
// suffix, e.g. "/api/recipients/" + "{id}" + "/contact". The first matching
// rule wins.
type rule struct {
	method string
	path   string
	exact  bool
	suffix string
	access Access
}

// rules is the route policy of the whole API (docs/bezpieczenstwo.md
// "Uprawnienia", ADR-0010). Senders compose and send messages: they read the
// group list, look recipients up by name (without contact data), resolve a
// selection and preview. Everything else about the recipient base, users,
// configuration and statistics is for administrators.
var rules = []rule{
	{path: "/healthz", exact: true, access: AccessPublic},
	{method: http.MethodPost, path: "/api/auth/login", exact: true, access: AccessPublic},
	// Operator webhooks authenticate with their own token or signature (#28).
	{path: "/providers/", access: AccessPublic},

	{method: http.MethodGet, path: "/api/groups", exact: true, access: AccessUser},
	{method: http.MethodPost, path: "/api/groups/resolve", exact: true, access: AccessUser},
	{path: "/api/groups", access: AccessAdmin},
	// The composer's search, without contact data (GET /recipients/lookup).
	{method: http.MethodGet, path: "/api/recipients/lookup", exact: true, access: AccessUser},
	// Filling in a missing contact from the composer, write-only (ADR-0013).
	{method: http.MethodPatch, path: "/api/recipients/", suffix: "/contact", access: AccessUser},
	{path: "/api/recipients", access: AccessAdmin},
	{path: "/api/users", access: AccessAdmin},
	{path: "/api/audit", access: AccessAdmin},
	{path: "/api/stats", access: AccessAdmin},
	{path: "/api/admin", access: AccessAdmin},
}

// RouteAccess returns who may call method on path.
func RouteAccess(method, path string) Access {
	for _, r := range rules {
		if r.method != "" && r.method != method {
			continue
		}
		if r.matches(path) {
			return r.access
		}
	}
	return AccessUser
}

func (r rule) matches(path string) bool {
	switch {
	case r.suffix != "":
		id, ok := strings.CutPrefix(path, r.path)
		if !ok {
			return false
		}
		id, ok = strings.CutSuffix(id, r.suffix)
		return ok && id != "" && !strings.Contains(id, "/")
	case r.exact:
		return path == r.path
	default:
		return hasPathPrefix(path, r.path)
	}
}

// hasPathPrefix matches whole path segments: "/api/users" covers
// "/api/users/1" but not "/api/usersfoo". A prefix ending in "/" matches as is.
func hasPathPrefix(path, prefix string) bool {
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(path, prefix)
	}
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}
