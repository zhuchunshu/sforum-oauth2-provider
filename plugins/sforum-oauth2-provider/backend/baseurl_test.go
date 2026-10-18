package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		" http://127.0.0.1:3000/ ":             "http://127.0.0.1:3000",
		"https://forum.example.com/forum/":     "https://forum.example.com/forum",
		"https://forum.example.com/p?x=1#frag": "https://forum.example.com/p",
		"javascript:alert(1)":                  "",
		"ftp://forum.example.com":              "",
		"":                                     "",
		"not a url":                            "",
	}
	for input, want := range cases {
		if got := normalizeBaseURL(input); got != want {
			t.Fatalf("normalizeBaseURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAbsoluteURL(t *testing.T) {
	cases := []struct {
		base  string
		value string
		want  string
	}{
		{"http://127.0.0.1:3000", "/media/avatars/abc", "http://127.0.0.1:3000/media/avatars/abc"},
		{"https://forum.example.com/forum", "/media/avatars/abc", "https://forum.example.com/media/avatars/abc"},
		{"http://127.0.0.1:3000", "data:image/svg+xml;base64,abc", "data:image/svg+xml;base64,abc"},
		{"http://127.0.0.1:3000", "https://cdn.example.com/a.png", "https://cdn.example.com/a.png"},
		{"http://127.0.0.1:3000", "//cdn.example.com/a.png", "http://cdn.example.com/a.png"},
		{"", "/media/avatars/abc", "/media/avatars/abc"},
		{"http://127.0.0.1:3000", "", ""},
	}
	for _, tc := range cases {
		if got := absoluteURL(tc.base, tc.value); got != tc.want {
			t.Fatalf("absoluteURL(%q, %q) = %q, want %q", tc.base, tc.value, got, tc.want)
		}
	}
}

func TestPublicBaseURLUsesRuntimeSiteURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != siteURLOptionPath {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"code":200,"message":"OK","data":{"name":"site.url","value":"https://forum.example.com/"}}`)
	}))
	t.Setenv("APP_URL", server.URL)

	oauth := testServer(t)
	if got := oauth.publicBaseURL(context.Background()); got != "https://forum.example.com" {
		t.Fatalf("publicBaseURL = %q, want the runtime site.url", got)
	}
	// 缓存命中：站点地址几乎不变，不应该每次 userinfo 都打一次公开选项接口。
	server.Close()
	if got := oauth.publicBaseURL(context.Background()); got != "https://forum.example.com" {
		t.Fatalf("publicBaseURL cache miss: %q", got)
	}
}

func TestPublicBaseURLFallsBackToAPPURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Setenv("APP_URL", server.URL+"/")

	oauth := testServer(t)
	if got := oauth.publicBaseURL(context.Background()); got != server.URL {
		t.Fatalf("publicBaseURL = %q, want APP_URL fallback %q", got, server.URL)
	}
	server.Close()
}

func TestPublicBaseURLWithoutAPPURL(t *testing.T) {
	t.Setenv("APP_URL", "")
	oauth := testServer(t)
	if got := oauth.publicBaseURL(context.Background()); got != "" {
		t.Fatalf("publicBaseURL = %q, want empty without APP_URL", got)
	}
}
