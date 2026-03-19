package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidUsername(t *testing.T) {
	valid := []string{
		"a", "ab", "a-b", "en9inerd", "user-123", "A", "a1",
		"abcdefghijklmnopqrstuvwxyz0123456789abc", // 39 chars
	}
	for _, u := range valid {
		if !ValidUsername(u) {
			t.Errorf("expected %q to be valid", u)
		}
	}

	invalid := []string{
		"", "-user", "user-", "-", "--", "a--b-",
		"user name", "user@name", "user.name",
		"abcdefghijklmnopqrstuvwxyz0123456789abcd", // 40 chars
		"!!!",
	}
	for _, u := range invalid {
		if ValidUsername(u) {
			t.Errorf("expected %q to be invalid", u)
		}
	}
}

func TestFetchUserDataSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/testuser", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(UserProfile{Login: "testuser", Name: "Test", PublicGists: 2})
	})
	mux.HandleFunc("GET /users/testuser/repos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Repo{{Name: "repo1", Language: "Go", Size: 100}})
	})
	mux.HandleFunc("GET /users/testuser/events", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Event{{Type: "PushEvent"}})
	})
	mux.HandleFunc("GET /users/testuser/orgs", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Org{{Login: "myorg"}})
	})
	mux.HandleFunc("GET /users/testuser/starred", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]json.RawMessage{[]byte(`{}`)})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient("")
	origFetch := fetchUserDataBaseURL
	fetchUserDataBaseURL = srv.URL
	defer func() { fetchUserDataBaseURL = origFetch }()

	data, err := client.FetchUserData(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Profile.Login != "testuser" {
		t.Errorf("expected login=testuser, got %s", data.Profile.Login)
	}
	if len(data.Repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(data.Repos))
	}
	if len(data.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(data.Events))
	}
	if len(data.Orgs) != 1 {
		t.Errorf("expected 1 org, got %d", len(data.Orgs))
	}
	if data.Gists != 2 {
		t.Errorf("expected 2 gists, got %d", data.Gists)
	}
	if data.Starred != 1 {
		t.Errorf("expected 1 starred, got %d", data.Starred)
	}
}

func TestFetchUserDataNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient("")
	origFetch := fetchUserDataBaseURL
	fetchUserDataBaseURL = srv.URL
	defer func() { fetchUserDataBaseURL = origFetch }()

	_, err := client.FetchUserData(context.Background(), "nobody")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestFetchUserDataRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient("")
	origFetch := fetchUserDataBaseURL
	fetchUserDataBaseURL = srv.URL
	defer func() { fetchUserDataBaseURL = origFetch }()

	_, err := client.FetchUserData(context.Background(), "someone")
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
}

func TestFetchStarredCountPaginated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<http://example.com?per_page=1&page=87>; rel="last"`)
		json.NewEncoder(w).Encode([]json.RawMessage{[]byte(`{}`)})
	}))
	defer srv.Close()

	client := NewClient("")
	origFetch := fetchUserDataBaseURL
	fetchUserDataBaseURL = srv.URL
	defer func() { fetchUserDataBaseURL = origFetch }()

	count, err := client.fetchStarredCount(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 87 {
		t.Errorf("expected 87, got %d", count)
	}
}

func TestFetchStarredCountNoLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]json.RawMessage{[]byte(`{}`), []byte(`{}`)})
	}))
	defer srv.Close()

	client := NewClient("")
	origFetch := fetchUserDataBaseURL
	fetchUserDataBaseURL = srv.URL
	defer func() { fetchUserDataBaseURL = origFetch }()

	count, err := client.fetchStarredCount(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient("ghp_testtoken123")
	origFetch := fetchUserDataBaseURL
	fetchUserDataBaseURL = srv.URL
	defer func() { fetchUserDataBaseURL = origFetch }()

	client.FetchUserData(context.Background(), "user")
	if gotAuth != "Bearer ghp_testtoken123" {
		t.Errorf("expected Bearer token, got %q", gotAuth)
	}
}

func TestCacheIntegration(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/cached", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(UserProfile{Login: "cached"})
	})
	mux.HandleFunc("GET /users/cached/repos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Repo{})
	})
	mux.HandleFunc("GET /users/cached/events", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Event{})
	})
	mux.HandleFunc("GET /users/cached/orgs", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Org{})
	})
	mux.HandleFunc("GET /users/cached/starred", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]json.RawMessage{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient("")
	origFetch := fetchUserDataBaseURL
	fetchUserDataBaseURL = srv.URL
	defer func() { fetchUserDataBaseURL = origFetch }()

	for range 5 {
		_, err := client.FetchUserData(context.Background(), "cached")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if callCount != 1 {
		t.Errorf("expected 1 API call (cached), got %d", callCount)
	}
}

func TestPagination(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			nextURL := fmt.Sprintf("http://%s%s?page=2", r.Host, r.URL.Path)
			w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
			json.NewEncoder(w).Encode([]Repo{{Name: "repo1"}})
		} else {
			json.NewEncoder(w).Encode([]Repo{{Name: "repo2"}})
		}
	}))
	defer srv.Close()

	client := NewClient("")
	repos, err := fetchAllPages[Repo](client, context.Background(), srv.URL+"/users/test/repos")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos from pagination, got %d", len(repos))
	}
}
