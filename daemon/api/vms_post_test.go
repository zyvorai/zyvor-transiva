// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMergeJSONBodyIntoQuery(t *testing.T) {
	body := `{"provider":"nutanix","server":"127.0.0.1:8990","username":"admin","password":"p@ss&word=1","insecure":true,"detailed":false,"ignored":{"a":1}}`
	r := httptest.NewRequest(http.MethodPost, "/vms/list?cluster=c1&username=fromurl", strings.NewReader(body))

	if err := mergeJSONBodyIntoQuery(r); err != nil {
		t.Fatal(err)
	}
	q := r.URL.Query()
	want := map[string]string{
		"provider": "nutanix", "server": "127.0.0.1:8990", "password": "p@ss&word=1",
		"insecure": "true", "detailed": "false", "cluster": "c1",
		"username": "fromurl", // a value already in the URL wins
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if q.Has("ignored") {
		t.Error("non-scalar fields must be ignored")
	}
}

func TestMergeJSONBodyIntoQueryRejectsBadJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/vms/list", strings.NewReader("not json"))
	if err := mergeJSONBodyIntoQuery(r); err == nil {
		t.Error("expected an error for a body that is not JSON")
	}
}
