package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type endpointResp struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

const (
	epURL    = "https://merchant-a.example/webhooks"
	epSecret = "whsec_super_secret_value"
)

// AC1/AC2/AC3/AC6: a session can create, list and disable its endpoints, and the secret is never
// returned.
func TestWebhookEndpoints_CreateListDisable(t *testing.T) {
	f := newTwoMerchants(t)

	w := f.get(http.MethodPost, "/v1/webhook_endpoints", f.aToken, `{"url":"`+epURL+`","secret":"`+epSecret+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), epSecret) || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("create response leaked the secret: %s", w.Body.String())
	}
	var created endpointResp
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == "" || created.URL != epURL || !created.Active {
		t.Fatalf("unexpected created endpoint: %+v", created)
	}

	list := f.get(http.MethodGet, "/v1/webhook_endpoints", f.aToken, "")
	if !strings.Contains(list.Body.String(), created.ID) {
		t.Fatalf("list is missing the new endpoint: %s", list.Body.String())
	}
	if strings.Contains(list.Body.String(), epSecret) {
		t.Fatalf("list leaked the secret: %s", list.Body.String())
	}

	dis := f.get(http.MethodPost, "/v1/webhook_endpoints/"+created.ID+"/disable", f.aToken, "")
	if dis.Code != http.StatusOK {
		t.Fatalf("disable: want 200, got %d", dis.Code)
	}
	var disabled endpointResp
	_ = json.Unmarshal(dis.Body.Bytes(), &disabled)
	if disabled.Active {
		t.Fatal("endpoint should be inactive after disable")
	}
}

// AC7: validation.
func TestWebhookEndpoints_Validation(t *testing.T) {
	f := newTwoMerchants(t)
	if w := f.get(http.MethodPost, "/v1/webhook_endpoints", f.aToken, `{"url":"not a url","secret":"x"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad url: want 400, got %d", w.Code)
	}
	if w := f.get(http.MethodPost, "/v1/webhook_endpoints", f.aToken, `{"url":"https://a.example","secret":""}`); w.Code != http.StatusBadRequest {
		t.Fatalf("empty secret: want 400, got %d", w.Code)
	}
}

// AC4: tenant isolation across the config writes.
func TestWebhookEndpoints_TenantIsolation(t *testing.T) {
	f := newTwoMerchants(t)

	w := f.get(http.MethodPost, "/v1/webhook_endpoints", f.aToken, `{"url":"`+epURL+`","secret":"`+epSecret+`"}`)
	var created endpointResp
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	// Sign in as merchant B.
	bl := f.get(http.MethodPost, "/v1/portal/login", "", `{"email":"b@test.example","password":"pw-abcdef"}`)
	var lr struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(bl.Body.Bytes(), &lr)

	if list := f.get(http.MethodGet, "/v1/webhook_endpoints", lr.Token, ""); strings.Contains(list.Body.String(), created.ID) {
		t.Fatalf("merchant B can see merchant A's endpoint: %s", list.Body.String())
	}
	if dis := f.get(http.MethodPost, "/v1/webhook_endpoints/"+created.ID+"/disable", lr.Token, ""); dis.Code != http.StatusNotFound {
		t.Fatalf("B disabling A's endpoint: want 404, got %d", dis.Code)
	}
}
