package models

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestNormalizeID(t *testing.T) {
	// aws:claude-opus-4.8, claude-opus-4-8 and claude-opus-4.8 all share a key.
	a := normalizeID("aws:claude-opus-4.8")
	b := normalizeID("claude-opus-4-8")
	c := normalizeID("claude-opus-4.8")
	if a != b || b != c {
		t.Errorf("normalizeID mismatch: %q %q %q", a, b, c)
	}

	// Date-pinned stays distinct from the plain form.
	if normalizeID("claude-haiku-4-5") == normalizeID("claude-haiku-4-5-20251001") {
		t.Error("date-pinned ID should normalize distinctly from the plain form")
	}
}

func TestDedup_PrefersDottedUnprefixed(t *testing.T) {
	in := []string{
		"aws:claude-opus-4.8",
		"claude-opus-4-8",
		"claude-opus-4.8",
		"claude-haiku-4-5-20251001",
		"gpt-5",
	}
	got := Dedup(in)
	want := []string{"claude-haiku-4-5-20251001", "claude-opus-4.8", "gpt-5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Dedup() = %v, want %v", got, want)
	}
}

func modelsHandler(ids []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[`)
		for i, id := range ids {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"id":%q}`, id)
		}
		fmt.Fprint(w, `]}`)
	}
}

func TestFetchModelIDs_V1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
				t.Errorf("auth header = %q, want Bearer sk-test", got)
			}
			modelsHandler([]string{"gpt-5", "claude-opus-4.8"})(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got, err := FetchModelIDs(srv.URL, "sk-test", nil)
	if err != nil {
		t.Fatalf("FetchModelIDs: %v", err)
	}
	want := []string{"claude-opus-4.8", "gpt-5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFetchModelIDs_FallbackToModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.WriteHeader(http.StatusNotFound)
		case "/models":
			modelsHandler([]string{"o3"})(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	got, err := FetchModelIDs(srv.URL, "sk-test", nil)
	if err != nil {
		t.Fatalf("FetchModelIDs: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"o3"}) {
		t.Errorf("got %v, want [o3]", got)
	}
}

func TestFetchModelIDs_BothFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := FetchModelIDs(srv.URL, "sk-test", nil); err == nil {
		t.Error("expected error when both endpoints fail")
	}
}

func TestFetchModelIDs_TrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			modelsHandler([]string{"gpt-5"})(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got, err := FetchModelIDs(srv.URL+"/", "sk-test", nil)
	if err != nil {
		t.Fatalf("FetchModelIDs: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"gpt-5"}) {
		t.Errorf("got %v, want [gpt-5]", got)
	}
}
