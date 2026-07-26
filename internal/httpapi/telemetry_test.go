package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClassifyNamespaceRequest(t *testing.T) {
	tests := []struct {
		method    string
		path      string
		namespace string
		operation string
		ok        bool
	}{
		{http.MethodGet, "/v1/tenant/overview", "tenant", "overview.get", true},
		{http.MethodPost, "/v1/tenant/query", "tenant", "query.run", true},
		{http.MethodGet, "/v1/tenant/tables/users/rows/42", "tenant", "rows.get", true},
		{http.MethodPatch, "/v1/tenant/tables/users/rows", "tenant", "rows.bulk_update", true},
		{http.MethodDelete, "/v1/tenant/tables/users/indexes/by_email", "tenant", "indexes.delete", true},
		{http.MethodPost, "/v1/namespaces/source/duplicate", "source", "namespaces.duplicate", true},
		{http.MethodGet, "/v1/namespaces", "", "", false},
		{http.MethodGet, "/healthz", "", "", false},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		namespace, operation, ok := classifyNamespaceRequest(request)
		if namespace != test.namespace || operation != test.operation || ok != test.ok {
			t.Errorf("%s %s = (%q, %q, %t), want (%q, %q, %t)",
				test.method, test.path, namespace, operation, ok,
				test.namespace, test.operation, test.ok)
		}
	}
}

func TestLegacyStatsEndpointIsRemoved(t *testing.T) {
	handler, _ := newAPIForTest(t)
	response := doReq(t, handler, http.MethodGet, "/v1/ws/stats", nil, nil)
	mustStatus(t, response, http.StatusNotFound)
}
