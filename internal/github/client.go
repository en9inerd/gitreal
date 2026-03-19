package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/en9inerd/gitreal/internal/cache"
)

type UserProfile struct {
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	AvatarURL   string    `json:"avatar_url"`
	HTMLURL     string    `json:"html_url"`
	Bio         string    `json:"bio"`
	Company     string    `json:"company"`
	Location    string    `json:"location"`
	Blog        string    `json:"blog"`
	Email       string    `json:"email"`
	Hireable    bool      `json:"hireable"`
	PublicRepos int       `json:"public_repos"`
	PublicGists int       `json:"public_gists"`
	Followers   int       `json:"followers"`
	Following   int       `json:"following"`
	CreatedAt   time.Time `json:"created_at"`
}

type Repo struct {
	Name            string    `json:"name"`
	Fork            bool      `json:"fork"`
	StargazersCount int       `json:"stargazers_count"`
	ForksCount      int       `json:"forks_count"`
	Description     string    `json:"description"`
	License         *License  `json:"license"`
	Language        string    `json:"language"`
	Size            int       `json:"size"`
	Topics          []string  `json:"topics"`
	Archived        bool      `json:"archived"`
	HasPages        bool      `json:"has_pages"`
	CreatedAt       time.Time `json:"created_at"`
	PushedAt        time.Time `json:"pushed_at"`
}

type License struct {
	SPDXID string `json:"spdx_id"`
}

type Event struct {
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Repo      EventRepo `json:"repo"`
}

type EventRepo struct {
	Name string `json:"name"`
}

type Org struct {
	Login string `json:"login"`
}

type UserData struct {
	Profile UserProfile
	Repos   []Repo
	Events  []Event
	Orgs    []Org
	Gists   int
	Starred int
}

type Client struct {
	httpClient *http.Client
	token      string
	cache      *cache.Cache[*UserData]
}

func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      token,
		cache:      cache.New[*UserData](5*time.Minute, 500),
	}
}

func (c *Client) FetchUserData(ctx context.Context, username string) (*UserData, error) {
	key := strings.ToLower(username)
	if cached, ok := c.cache.Get(key); ok {
		return cached, nil
	}

	data := &UserData{}

	profile, err := fetchJSON[UserProfile](c, ctx, fmt.Sprintf("%s/users/%s", fetchUserDataBaseURL, username))
	if err != nil {
		return nil, fmt.Errorf("user not found or API error: %w", err)
	}
	data.Profile = *profile

	repos, err := fetchAllPages[Repo](c, ctx, fmt.Sprintf("%s/users/%s/repos?per_page=100&sort=created", fetchUserDataBaseURL, username))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repos: %w", err)
	}
	data.Repos = repos

	events, err := fetchJSON[[]Event](c, ctx, fmt.Sprintf("%s/users/%s/events?per_page=100", fetchUserDataBaseURL, username))
	if err == nil {
		data.Events = *events
	}

	orgs, err := fetchJSON[[]Org](c, ctx, fmt.Sprintf("%s/users/%s/orgs", fetchUserDataBaseURL, username))
	if err == nil {
		data.Orgs = *orgs
	}

	data.Gists = data.Profile.PublicGists

	starred, err := c.fetchStarredCount(ctx, username)
	if err == nil {
		data.Starred = starred
	}

	c.cache.Set(key, data)
	return data, nil
}

func (c *Client) newRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gitreal")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

func fetchJSON[T any](c *Client, ctx context.Context, url string) (*T, error) {
	req, err := c.newRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("rate limited by GitHub API")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

var fetchUserDataBaseURL = "https://api.github.com"

func SetBaseURL(url string) { fetchUserDataBaseURL = url }

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`)

func ValidUsername(username string) bool {
	return usernameRe.MatchString(username)
}

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)
var linkLastRe = regexp.MustCompile(`<([^>]+)>;\s*rel="last"`)

func fetchAllPages[T any](c *Client, ctx context.Context, url string) ([]T, error) {
	var all []T
	currentURL := url
	for currentURL != "" {
		req, err := c.newRequest(ctx, currentURL)
		if err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}
		var page []T
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()
		all = append(all, page...)

		linkHeader := resp.Header.Get("Link")
		if matches := linkNextRe.FindStringSubmatch(linkHeader); len(matches) > 1 {
			currentURL = matches[1]
		} else {
			currentURL = ""
		}
	}
	return all, nil
}

func (c *Client) fetchStarredCount(ctx context.Context, username string) (int, error) {
	url := fmt.Sprintf("%s/users/%s/starred?per_page=1", fetchUserDataBaseURL, username)
	req, err := c.newRequest(ctx, url)
	if err != nil {
		return 0, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	linkHeader := resp.Header.Get("Link")
	if linkHeader == "" {
		var items []json.RawMessage
		json.NewDecoder(resp.Body).Decode(&items)
		return len(items), nil
	}
	if matches := linkLastRe.FindStringSubmatch(linkHeader); len(matches) > 1 {
		lastURL := matches[1]
		for _, prefix := range []string{"&page=", "?page="} {
			if idx := strings.Index(lastURL, prefix); idx != -1 {
				pageStr := lastURL[idx+len(prefix):]
				if ampIdx := strings.Index(pageStr, "&"); ampIdx != -1 {
					pageStr = pageStr[:ampIdx]
				}
				if count, err := strconv.Atoi(pageStr); err == nil {
					return count, nil
				}
			}
		}
	}
	return 0, nil
}
