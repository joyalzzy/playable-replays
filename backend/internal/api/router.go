package api

import (
	"net/http"
	"sort"
	"strings"
)

type routeDefinition struct {
	method   string
	segments []string
	handler  http.HandlerFunc
}

type structuredRouter struct {
	routes []routeDefinition
}

func (r *structuredRouter) handle(method, path string, handler http.HandlerFunc) {
	r.routes = append(r.routes, routeDefinition{method: method, segments: routeSegments(path), handler: handler})
}

func (r *structuredRouter) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	requestSegments := routeSegments(request.URL.Path)
	allowed := []string{}
	for _, route := range r.routes {
		values, matches := matchRoute(route.segments, requestSegments)
		if !matches {
			continue
		}
		allowed = append(allowed, route.method)
		if route.method != request.Method {
			continue
		}
		for key, value := range values {
			request.SetPathValue(key, value)
		}
		route.handler(w, request)
		return
	}
	if len(allowed) == 0 {
		writeError(w, http.StatusNotFound, "route_not_found", "the requested API route does not exist")
		return
	}
	sort.Strings(allowed)
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "the requested method is not allowed for this route")
}

func routeSegments(path string) []string {
	if path == "/" {
		return nil
	}
	if !strings.HasPrefix(path, "/") {
		return []string{path}
	}
	return strings.Split(path[1:], "/")
}

func matchRoute(pattern, actual []string) (map[string]string, bool) {
	if len(pattern) != len(actual) {
		return nil, false
	}
	values := map[string]string{}
	for index, segment := range pattern {
		value := actual[index]
		if len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
			if value == "" {
				return nil, false
			}
			values[segment[1:len(segment)-1]] = value
			continue
		}
		if segment != value {
			return nil, false
		}
	}
	return values, true
}
