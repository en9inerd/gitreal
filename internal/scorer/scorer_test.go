package scorer

import (
	"testing"
	"time"

	"github.com/en9inerd/gitreal/internal/github"
)

func makeProfile(ageMonths int) github.UserProfile {
	return github.UserProfile{
		Login:     "testuser",
		Name:      "Test User",
		AvatarURL: "https://example.com/avatar.png",
		HTMLURL:   "https://github.com/testuser",
		Bio:       "A developer",
		Company:   "TestCo",
		Location:  "Earth",
		Blog:      "https://example.com",
		Followers: 10,
		Following: 20,
		CreatedAt: time.Now().AddDate(0, -ageMonths, 0),
	}
}

func makeRepo(name, lang string, stars, forks, size int, fork bool) github.Repo {
	return github.Repo{
		Name:            name,
		Language:        lang,
		StargazersCount: stars,
		ForksCount:      forks,
		Size:            size,
		Fork:            fork,
		Description:     "A repo",
		License:         &github.License{SPDXID: "MIT"},
		Topics:          []string{"go"},
		CreatedAt:       time.Now().AddDate(-2, 0, 0),
		PushedAt:        time.Now().AddDate(0, -1, 0),
	}
}

func TestCalculateVerdicts(t *testing.T) {
	tests := []struct {
		name    string
		data    *github.UserData
		wantMin int
		wantMax int
		verdict string
	}{
		{
			name: "empty account",
			data: &github.UserData{
				Profile: github.UserProfile{CreatedAt: time.Now()},
			},
			wantMin: 0,
			wantMax: 5,
			verdict: "Likely Fake",
		},
		{
			name: "veteran developer",
			data: &github.UserData{
				Profile: makeProfile(120),
				Repos: []github.Repo{
					makeRepo("repo1", "Go", 20, 5, 500, false),
					makeRepo("repo2", "Python", 15, 3, 300, false),
					makeRepo("repo3", "Rust", 10, 2, 200, false),
					makeRepo("repo4", "JS", 8, 1, 150, false),
					makeRepo("repo5", "TS", 5, 1, 100, false),
					makeRepo("repo6", "C", 3, 0, 80, false),
				},
				Events: makeEvents(60, 6),
				Orgs:   []github.Org{{Login: "org1"}, {Login: "org2"}},
				Gists:  5,
				Starred: 100,
			},
			wantMin: 70,
			wantMax: 100,
			verdict: "Likely Real",
		},
		{
			name: "low confidence student",
			data: &github.UserData{
				Profile: github.UserProfile{
					Login:     "student",
					Name:      "Student",
					CreatedAt: time.Now().AddDate(0, -3, 0),
					Followers: 1,
				},
				Repos: []github.Repo{
					makeRepo("hw1", "Python", 0, 0, 20, false),
				},
				Starred: 5,
			},
			wantMin: 15,
			wantMax: 49,
			verdict: "Low Confidence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Calculate(tt.data)
			if result.Total < tt.wantMin || result.Total > tt.wantMax {
				t.Errorf("total=%d, want [%d, %d]", result.Total, tt.wantMin, tt.wantMax)
			}
			if result.Verdict != tt.verdict {
				t.Errorf("verdict=%q, want %q (total=%d)", result.Verdict, tt.verdict, result.Total)
			}
			if result.MaxTotal != 100 {
				t.Errorf("max_total=%d, want 100", result.MaxTotal)
			}
		})
	}
}

func TestCategoryMaxScores(t *testing.T) {
	data := &github.UserData{Profile: makeProfile(120)}
	result := Calculate(data)

	expected := map[string]int{
		"Account Age":  15,
		"Profile":      10,
		"Repositories": 20,
		"Activity":     25,
		"Social Proof": 15,
		"Diversity":    15,
	}

	categories := result.Categories()
	if len(categories) != 6 {
		t.Fatalf("expected 6 categories, got %d", len(categories))
	}

	maxSum := 0
	for _, cat := range categories {
		want, ok := expected[cat.Name]
		if !ok {
			t.Errorf("unexpected category %q", cat.Name)
			continue
		}
		if cat.Score.MaxScore != want {
			t.Errorf("%s: max_score=%d, want %d", cat.Name, cat.Score.MaxScore, want)
		}
		maxSum += cat.Score.MaxScore
	}
	if maxSum != 100 {
		t.Errorf("sum of max_scores=%d, want 100", maxSum)
	}
}

func TestCategoryScoresNeverExceedMax(t *testing.T) {
	data := &github.UserData{
		Profile: makeProfile(120),
		Repos: func() []github.Repo {
			var repos []github.Repo
			langs := []string{"Go", "Python", "Rust", "JS", "TS", "C", "Java", "Ruby"}
			for i, l := range langs {
				repos = append(repos, makeRepo("repo"+string(rune('a'+i)), l, 50, 10, 1000, false))
			}
			return repos
		}(),
		Events:  makeEvents(100, 8),
		Orgs:    []github.Org{{Login: "a"}, {Login: "b"}, {Login: "c"}, {Login: "d"}},
		Gists:   10,
		Starred: 200,
	}

	result := Calculate(data)
	for _, cat := range result.Categories() {
		if cat.Score.Score > cat.Score.MaxScore {
			t.Errorf("%s: score %d exceeds max %d", cat.Name, cat.Score.Score, cat.Score.MaxScore)
		}
		if cat.Score.Score < 0 {
			t.Errorf("%s: negative score %d", cat.Name, cat.Score.Score)
		}
		for _, d := range cat.Score.Details {
			if d.Score > d.Max {
				t.Errorf("%s/%s: detail score %d exceeds max %d", cat.Name, d.Label, d.Score, d.Max)
			}
			if d.Score < 0 {
				t.Errorf("%s/%s: negative detail score %d", cat.Name, d.Label, d.Score)
			}
		}
	}

	if result.Total > 100 {
		t.Errorf("total %d exceeds 100", result.Total)
	}
}

func TestAccountAge(t *testing.T) {
	const daysPerHalfYear = 183 // ~6 months in days (6 * 30.44 ≈ 182.6)

	tests := []struct {
		days int
		want int
	}{
		{0, 0},
		{100, 0},                     // ~3.3 months
		{daysPerHalfYear, 1},         // 6 months
		{daysPerHalfYear * 2, 2},     // 1 year
		{daysPerHalfYear * 6, 6},     // 3 years
		{daysPerHalfYear * 10, 10},   // 5 years
		{daysPerHalfYear * 15, 15},   // 7.5 years
		{daysPerHalfYear * 20, 15},   // 10 years, capped at 15
	}

	for _, tt := range tests {
		data := &github.UserData{Profile: github.UserProfile{
			CreatedAt: time.Now().Add(-time.Duration(tt.days) * 24 * time.Hour),
		}}
		result := Calculate(data)
		if result.AccountAge.Score != tt.want {
			t.Errorf("days=%d: got %d, want %d", tt.days, result.AccountAge.Score, tt.want)
		}
	}
}

func TestProfileCompleteness(t *testing.T) {
	tests := []struct {
		name string
		prof github.UserProfile
		want int
	}{
		{"empty", github.UserProfile{}, 0},
		{"name only", github.UserProfile{Name: "Test"}, 2},
		{"all fields", github.UserProfile{Name: "T", Bio: "B", Location: "L", Blog: "W", Company: "C"}, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &github.UserData{Profile: tt.prof}
			result := Calculate(data)
			if result.ProfileCompleteness.Score != tt.want {
				t.Errorf("got %d, want %d", result.ProfileCompleteness.Score, tt.want)
			}
		})
	}
}

func TestRepoQualityZeroRepos(t *testing.T) {
	data := &github.UserData{Profile: github.UserProfile{}}
	result := Calculate(data)
	if result.RepoQuality.Score != 0 {
		t.Errorf("expected 0 for no repos, got %d", result.RepoQuality.Score)
	}
	if len(result.RepoQuality.Details) != 1 || result.RepoQuality.Details[0].Label != "No repositories" {
		t.Error("expected 'No repositories' detail")
	}
}

func TestRepoQualityAllForks(t *testing.T) {
	data := &github.UserData{
		Profile: github.UserProfile{},
		Repos: []github.Repo{
			makeRepo("fork1", "Go", 0, 0, 100, true),
			makeRepo("fork2", "Python", 0, 0, 100, true),
		},
	}
	result := Calculate(data)
	// original ratio = 0% → 0 for orig metric, nontrivial = 0 (forks excluded)
	for _, d := range result.RepoQuality.Details {
		if d.Label == "Original repos: 0/2 (0%)" && d.Score != 0 {
			t.Errorf("expected 0 for orig ratio with all forks, got %d", d.Score)
		}
	}
}

func TestActivityConsistencyNoData(t *testing.T) {
	data := &github.UserData{Profile: github.UserProfile{}}
	result := Calculate(data)
	if result.ActivityConsistency.Score != 0 {
		t.Errorf("expected 0 for no repos+events, got %d", result.ActivityConsistency.Score)
	}
}

func TestActivityConsistencyEventsOnly(t *testing.T) {
	data := &github.UserData{
		Profile: github.UserProfile{},
		Events:  makeEvents(20, 3),
	}
	result := Calculate(data)
	// span/years/months = 0 (from repos), but events give points
	if result.ActivityConsistency.Score == 0 {
		t.Error("expected some score from events alone")
	}
}

func TestSocialProofFollowerRatio(t *testing.T) {
	tests := []struct {
		name      string
		followers int
		following int
		wantMin   int
	}{
		{"zero", 0, 0, 0},
		{"organic", 10, 0, 5},  // ratio=10.0, followers≥5 → ratio=3, count=3
		{"healthy", 10, 5, 5},  // ratio=2.0, followers≥5 → ratio=3, count=3
		{"spammy", 2, 1000, 2}, // ratio=0.002, followers≥1 → ratio=1, count=1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &github.UserData{
				Profile: github.UserProfile{
					Followers: tt.followers,
					Following: tt.following,
				},
			}
			result := Calculate(data)
			if result.SocialProof.Score < tt.wantMin {
				t.Errorf("got %d, want >= %d", result.SocialProof.Score, tt.wantMin)
			}
		})
	}
}

func TestContributionDiversityExternalRepos(t *testing.T) {
	data := &github.UserData{
		Profile: github.UserProfile{Login: "alice"},
		Events: []github.Event{
			{Type: "PushEvent", Repo: github.EventRepo{Name: "alice/myrepo"}, CreatedAt: time.Now()},
			{Type: "IssuesEvent", Repo: github.EventRepo{Name: "bob/project"}, CreatedAt: time.Now()},
			{Type: "PullRequestEvent", Repo: github.EventRepo{Name: "org/lib"}, CreatedAt: time.Now()},
		},
	}
	result := Calculate(data)

	found := false
	for _, d := range result.ContributionDiversity.Details {
		if d.Label == "External repos: 2" {
			found = true
			if d.Score != 2 {
				t.Errorf("expected score 2 for 2 external repos, got %d", d.Score)
			}
		}
	}
	if !found {
		t.Error("external repos detail not found")
	}
}

func TestContributionDiversityEcosystem(t *testing.T) {
	tests := []struct {
		name    string
		gists   int
		starred int
		want    int
	}{
		{"neither", 0, 0, 0},
		{"starred only", 0, 10, 3},
		{"gists only", 5, 0, 3},
		{"both", 5, 10, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &github.UserData{
				Profile: github.UserProfile{},
				Gists:   tt.gists,
				Starred: tt.starred,
			}
			result := Calculate(data)

			found := false
			for _, d := range result.ContributionDiversity.Details {
				if d.Max == 5 && (d.Label == "Ecosystem participation" ||
					d.Label == "Ecosystem participation (gists + starred)" ||
					d.Label == "Ecosystem participation (gists)" ||
					d.Label == "Ecosystem participation (starred repos)") {
					found = true
					if d.Score != tt.want {
						t.Errorf("got %d, want %d", d.Score, tt.want)
					}
				}
			}
			if !found {
				t.Error("ecosystem detail not found")
			}
		})
	}
}

func TestTierScore(t *testing.T) {
	tiers := []tier{{50, 5}, {20, 4}, {10, 3}, {5, 2}, {1, 1}}

	tests := []struct {
		value int
		want  int
	}{
		{0, 0},
		{1, 1},
		{4, 1},
		{5, 2},
		{9, 2},
		{10, 3},
		{19, 3},
		{20, 4},
		{49, 4},
		{50, 5},
		{1000, 5},
	}

	for _, tt := range tests {
		got := tierScore(tt.value, tiers)
		if got != tt.want {
			t.Errorf("tierScore(%d)=%d, want %d", tt.value, got, tt.want)
		}
	}
}

func TestTotalMatchesSumOfCategories(t *testing.T) {
	data := &github.UserData{
		Profile: makeProfile(60),
		Repos:   []github.Repo{makeRepo("r1", "Go", 5, 1, 200, false)},
		Events:  makeEvents(10, 3),
		Orgs:    []github.Org{{Login: "org1"}},
		Gists:   2,
		Starred: 30,
	}

	result := Calculate(data)
	sum := 0
	for _, cat := range result.Categories() {
		sum += cat.Score.Score
	}
	if result.Total != sum {
		t.Errorf("total=%d, but category sum=%d", result.Total, sum)
	}
}

func makeEvents(count, types int) []github.Event {
	eventTypes := []string{
		"PushEvent", "CreateEvent", "IssuesEvent",
		"PullRequestEvent", "WatchEvent", "ForkEvent",
		"IssueCommentEvent", "PullRequestReviewEvent",
	}
	events := make([]github.Event, count)
	for i := range count {
		events[i] = github.Event{
			Type:      eventTypes[i%types],
			CreatedAt: time.Now().Add(-time.Duration(i) * 24 * time.Hour),
			Repo: github.EventRepo{
				Name: func() string {
					if i%3 == 0 {
						return "other/repo"
					}
					return "testuser/myrepo"
				}(),
			},
		}
	}
	return events
}
