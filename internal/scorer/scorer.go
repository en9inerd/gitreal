package scorer

import (
	"fmt"
	"strings"
	"time"

	"github.com/en9inerd/gitreal/internal/github"
)

type ScoreResult struct {
	Username              string        `json:"username"`
	AvatarURL             string        `json:"avatar_url"`
	ProfileURL            string        `json:"profile_url"`
	Name                  string        `json:"name"`
	Total                 int           `json:"total"`
	MaxTotal              int           `json:"max_total"`
	Verdict               string        `json:"verdict"`
	AccountAge            CategoryScore `json:"account_age"`
	ProfileCompleteness   CategoryScore `json:"profile_completeness"`
	RepoQuality           CategoryScore `json:"repo_quality"`
	ActivityConsistency   CategoryScore `json:"activity_consistency"`
	SocialProof           CategoryScore `json:"social_proof"`
	ContributionDiversity CategoryScore `json:"contribution_diversity"`
}

type CategoryScore struct {
	Score    int           `json:"score"`
	MaxScore int           `json:"max_score"`
	Details  []ScoreDetail `json:"details"`
}

type ScoreDetail struct {
	Label string `json:"label"`
	Score int    `json:"score"`
	Max   int    `json:"max"`
}

type NamedCategory struct {
	Name  string
	Icon  string
	Score CategoryScore
}

func (r *ScoreResult) Categories() []NamedCategory {
	return []NamedCategory{
		{Name: "Account Age", Icon: "📅", Score: r.AccountAge},
		{Name: "Profile", Icon: "👤", Score: r.ProfileCompleteness},
		{Name: "Repositories", Icon: "📦", Score: r.RepoQuality},
		{Name: "Activity", Icon: "📊", Score: r.ActivityConsistency},
		{Name: "Social Proof", Icon: "🤝", Score: r.SocialProof},
		{Name: "Diversity", Icon: "🌐", Score: r.ContributionDiversity},
	}
}

func Calculate(data *github.UserData) *ScoreResult {
	result := &ScoreResult{
		Username:   data.Profile.Login,
		AvatarURL:  data.Profile.AvatarURL,
		ProfileURL: data.Profile.HTMLURL,
		Name:       data.Profile.Name,
		MaxTotal:   100,
	}

	result.AccountAge = calcAccountAge(data)
	result.ProfileCompleteness = calcProfileCompleteness(data)
	result.RepoQuality = calcRepoQuality(data)
	result.ActivityConsistency = calcActivityConsistency(data)
	result.SocialProof = calcSocialProof(data)
	result.ContributionDiversity = calcContributionDiversity(data)

	result.Total = result.AccountAge.Score +
		result.ProfileCompleteness.Score +
		result.RepoQuality.Score +
		result.ActivityConsistency.Score +
		result.SocialProof.Score +
		result.ContributionDiversity.Score

	switch {
	case result.Total >= 50:
		result.Verdict = "Likely Real"
	case result.Total >= 20:
		result.Verdict = "Low Confidence"
	default:
		result.Verdict = "Likely Fake"
	}

	return result
}

func calcAccountAge(data *github.UserData) CategoryScore {
	const maxScore = 15
	months := time.Since(data.Profile.CreatedAt).Hours() / (24 * 30.44)
	halfYears := int(months / 6)
	score := min(maxScore, halfYears)
	years := months / 12

	return CategoryScore{
		Score:    score,
		MaxScore: maxScore,
		Details: []ScoreDetail{
			{Label: fmt.Sprintf("Account is %.1f years old", years), Score: score, Max: maxScore},
		},
	}
}

func calcProfileCompleteness(data *github.UserData) CategoryScore {
	const maxScore = 10
	var details []ScoreDetail
	total := 0

	fields := []struct {
		label  string
		filled bool
	}{
		{"Name", data.Profile.Name != ""},
		{"Bio", data.Profile.Bio != ""},
		{"Location", data.Profile.Location != ""},
		{"Website", data.Profile.Blog != ""},
		{"Company", data.Profile.Company != ""},
	}

	for _, f := range fields {
		s := 0
		if f.filled {
			s = 2
		}
		total += s
		details = append(details, ScoreDetail{Label: f.label, Score: s, Max: 2})
	}

	return CategoryScore{Score: min(maxScore, total), MaxScore: maxScore, Details: details}
}

func calcRepoQuality(data *github.UserData) CategoryScore {
	const maxScore = 20
	totalRepos := len(data.Repos)
	if totalRepos == 0 {
		return CategoryScore{MaxScore: maxScore, Details: []ScoreDetail{{Label: "No repositories", Max: maxScore}}}
	}

	var original, withStars, totalStars, totalForks, withDesc, withLicense, withTopics, nontrivial int
	languages := map[string]bool{}

	for _, r := range data.Repos {
		if !r.Fork {
			original++
			if r.Size > 10 {
				nontrivial++
			}
		}
		if r.StargazersCount > 0 {
			withStars++
		}
		totalStars += r.StargazersCount
		totalForks += r.ForksCount
		if r.Description != "" {
			withDesc++
		}
		if r.License != nil && r.License.SPDXID != "" && r.License.SPDXID != "NOASSERTION" {
			withLicense++
		}
		if len(r.Topics) > 0 {
			withTopics++
		}
		if r.Language != "" {
			languages[r.Language] = true
		}
	}

	origRatio := float64(original) / float64(totalRepos)
	descRatio := float64(withDesc) / float64(totalRepos)
	licenseRatio := float64(withLicense) / float64(totalRepos)

	var details []ScoreDetail
	total := 0

	s := tierScore(int(origRatio*100), []tier{{80, 3}, {60, 2}, {40, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Original repos: %d/%d (%.0f%%)", original, totalRepos, origRatio*100), Score: s, Max: 3})
	total += s

	s = tierScore(withStars, []tier{{10, 3}, {5, 2}, {2, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Repos with stars: %d", withStars), Score: s, Max: 3})
	total += s

	s = tierScore(totalStars, []tier{{100, 2}, {50, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Total stars: %d", totalStars), Score: s, Max: 2})
	total += s

	s = tierScore(totalForks, []tier{{20, 2}, {5, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Total forks received: %d", totalForks), Score: s, Max: 2})
	total += s

	s = tierScore(int(descRatio*100), []tier{{90, 2}, {50, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("With description: %d/%d", withDesc, totalRepos), Score: s, Max: 2})
	total += s

	s = tierScore(int(licenseRatio*100), []tier{{70, 2}, {40, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("With license: %d/%d", withLicense, totalRepos), Score: s, Max: 2})
	total += s

	s = tierScore(len(languages), []tier{{5, 2}, {3, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Languages: %d", len(languages)), Score: s, Max: 2})
	total += s

	s = tierScore(nontrivial, []tier{{10, 2}, {5, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Non-trivial repos: %d", nontrivial), Score: s, Max: 2})
	total += s

	s = tierScore(withTopics, []tier{{5, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Repos with topics: %d", withTopics), Score: s, Max: 2})
	total += s

	return CategoryScore{Score: min(maxScore, total), MaxScore: maxScore, Details: details}
}

func calcActivityConsistency(data *github.UserData) CategoryScore {
	const maxScore = 25
	if len(data.Repos) == 0 && len(data.Events) == 0 {
		return CategoryScore{MaxScore: maxScore}
	}

	var earliest, latest time.Time
	activeYears := map[int]bool{}
	activeMonths := map[string]bool{}

	for i, r := range data.Repos {
		if i == 0 {
			earliest = r.CreatedAt
			latest = r.PushedAt
		}
		if r.CreatedAt.Before(earliest) {
			earliest = r.CreatedAt
		}
		if r.PushedAt.After(latest) {
			latest = r.PushedAt
		}
		activeYears[r.CreatedAt.Year()] = true
		activeYears[r.PushedAt.Year()] = true
		activeMonths[r.PushedAt.Format("2006-01")] = true
		activeMonths[r.CreatedAt.Format("2006-01")] = true
	}

	spanYears := 0.0
	if !earliest.IsZero() && !latest.IsZero() {
		spanYears = latest.Sub(earliest).Hours() / (24 * 365.25)
	}

	var details []ScoreDetail
	total := 0

	s := tierScore(int(spanYears*10), []tier{{50, 7}, {40, 6}, {30, 5}, {20, 4}, {10, 3}, {5, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Activity span: %.1f years", spanYears), Score: s, Max: 7})
	total += s

	s = tierScore(len(activeYears), []tier{{5, 5}, {4, 4}, {3, 3}, {2, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Active years: %d", len(activeYears)), Score: s, Max: 5})
	total += s

	s = tierScore(len(activeMonths), []tier{{24, 3}, {12, 2}, {4, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Active months: %d", len(activeMonths)), Score: s, Max: 3})
	total += s

	eventCount := len(data.Events)
	s = tierScore(eventCount, []tier{{50, 5}, {20, 4}, {10, 3}, {5, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Recent events: %d", eventCount), Score: s, Max: 5})
	total += s

	eventSpanDays := 0
	if len(data.Events) > 1 {
		first := data.Events[len(data.Events)-1].CreatedAt
		last := data.Events[0].CreatedAt
		eventSpanDays = int(last.Sub(first).Hours() / 24)
	}
	s = tierScore(eventSpanDays, []tier{{60, 5}, {30, 4}, {14, 3}, {7, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Recent activity span: %d days", eventSpanDays), Score: s, Max: 5})
	total += s

	return CategoryScore{Score: min(maxScore, total), MaxScore: maxScore, Details: details}
}

func calcSocialProof(data *github.UserData) CategoryScore {
	const maxScore = 15
	var details []ScoreDetail
	total := 0

	s := tierScore(data.Profile.Followers, []tier{{50, 5}, {20, 4}, {10, 3}, {5, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Followers: %d", data.Profile.Followers), Score: s, Max: 5})
	total += s

	ratio := 0.0
	if data.Profile.Following > 0 {
		ratio = float64(data.Profile.Followers) / float64(data.Profile.Following)
	} else if data.Profile.Followers > 0 {
		ratio = float64(data.Profile.Followers)
	}
	switch {
	case ratio >= 1.0 && data.Profile.Followers >= 5:
		s = 3
	case ratio >= 0.5 && data.Profile.Followers >= 2:
		s = 2
	case data.Profile.Followers >= 1:
		s = 1
	default:
		s = 0
	}
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Follower ratio: %.1f", ratio), Score: s, Max: 3})
	total += s

	s = tierScore(data.Starred, []tier{{50, 2}, {10, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Starred repos: %d", data.Starred), Score: s, Max: 2})
	total += s

	s = tierScore(len(data.Orgs), []tier{{3, 3}, {2, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Organizations: %d", len(data.Orgs)), Score: s, Max: 3})
	total += s

	s = tierScore(data.Gists, []tier{{3, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Public gists: %d", data.Gists), Score: s, Max: 2})
	total += s

	return CategoryScore{Score: min(maxScore, total), MaxScore: maxScore, Details: details}
}

func calcContributionDiversity(data *github.UserData) CategoryScore {
	const maxScore = 15
	var details []ScoreDetail
	total := 0

	eventTypes := map[string]bool{}
	for _, e := range data.Events {
		eventTypes[e.Type] = true
	}
	s := tierScore(len(eventTypes), []tier{{6, 5}, {4, 4}, {3, 3}, {2, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("Event types: %d", len(eventTypes)), Score: s, Max: 5})
	total += s

	username := data.Profile.Login
	externalRepos := map[string]bool{}
	for _, e := range data.Events {
		parts := strings.SplitN(e.Repo.Name, "/", 2)
		if len(parts) == 2 && !strings.EqualFold(parts[0], username) {
			externalRepos[e.Repo.Name] = true
		}
	}
	s = tierScore(len(externalRepos), []tier{{5, 5}, {4, 4}, {3, 3}, {2, 2}, {1, 1}})
	details = append(details, ScoreDetail{Label: fmt.Sprintf("External repos: %d", len(externalRepos)), Score: s, Max: 5})
	total += s

	hasGists := data.Gists > 0
	hasStarred := data.Starred > 0
	label := "Ecosystem participation"
	switch {
	case hasGists && hasStarred:
		s = 5
		label += " (gists + starred)"
	case hasGists:
		s = 3
		label += " (gists)"
	case hasStarred:
		s = 3
		label += " (starred repos)"
	default:
		s = 0
	}
	details = append(details, ScoreDetail{Label: label, Score: s, Max: 5})
	total += s

	return CategoryScore{Score: min(maxScore, total), MaxScore: maxScore, Details: details}
}

type tier struct {
	threshold int
	score     int
}

func tierScore(value int, tiers []tier) int {
	for _, t := range tiers {
		if value >= t.threshold {
			return t.score
		}
	}
	return 0
}
