package jira_account

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWhoAmI_AsksJiraWithBasicAuthAndReturnsTheAccount(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"displayName":"Yusuf Wijaya","emailAddress":"yusuf.wijaya@esb.co.id","accountId":"63f3","active":true}`))
	}))
	defer server.Close()

	who, err := NewChecker(server.URL).WhoAmI(context.Background(), "yusuf.wijaya@esb.co.id", "api-token")
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	if who.DisplayName != "Yusuf Wijaya" || who.AccountID != "63f3" || !who.Active {
		t.Errorf("identity = %+v", who)
	}
	if gotPath != "/rest/api/3/myself" {
		t.Errorf("path = %q", gotPath)
	}
	if want := "Basic " + base64.StdEncoding.EncodeToString([]byte("yusuf.wijaya@esb.co.id:api-token")); gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

func TestWhoAmI_MapsAuthFailuresToErrRejectedAndKeepsOtherErrorsApart(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		_, err := NewChecker(server.URL).WhoAmI(context.Background(), "a@esb.co.id", "secret-token")
		server.Close()
		if !errors.Is(err, ErrRejected) {
			t.Errorf("HTTP %d: err = %v, want ErrRejected", code, err)
		}
		if err != nil && strings.Contains(err.Error(), "secret-token") {
			t.Errorf("HTTP %d: the error must not contain the token: %v", code, err)
		}
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer down.Close()
	if _, err := NewChecker(down.URL).WhoAmI(context.Background(), "a@esb.co.id", "t"); err == nil || errors.Is(err, ErrRejected) {
		t.Errorf("a Jira outage must be an error that is not ErrRejected, got %v", err)
	}
}
