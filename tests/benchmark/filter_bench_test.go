package benchmark

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/filter"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

func BenchmarkFilterMatching(b *testing.B) {
	repos := make([]models.Repository, 1000)
	for i := range repos {
		repos[i] = models.Repository{
			ID: "r" + string(rune(i)), Owner: "owner", Name: "repo",
			HTTPSURL: "https://github.com/owner/repo",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sync.ApplyFilter(repos, sync.SyncFilter{Org: "owner"}, nil)
	}
}

func BenchmarkRepoignoreMatch(b *testing.B) {
	r, _ := filter.ParseRepoignoreFile("")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Match("https://github.com/owner/repo")
	}
}

func BenchmarkURLNormalization(b *testing.B) {
	urls := make([]string, 1000)
	for i := range urls {
		// Generate varying URL patterns
		switch i % 4 {
		case 0:
			urls[i] = "git@github.com:owner/repo.git"
		case 1:
			urls[i] = "https://github.com/owner/repo"
		case 2:
			urls[i] = "ssh://git@github.com/owner/repo.git"
		case 3:
			urls[i] = "git@gitlab.com:owner/repo"
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, url := range urls {
			utils.NormalizeHTTPS(url)
		}
	}
}

func BenchmarkMirrorDetection(b *testing.B) {
	// Create 1000 repos, some are mirrors (same owner/name across services)
	repos := make([]models.Repository, 1000)
	for i := range repos {
		serviceIdx := i % 4
		service := []string{"github", "gitlab", "gitflic", "gitverse"}[serviceIdx]
		owner := "owner" + string(rune(i%10))
		name := "repo" + string(rune(i%100))
		repos[i] = models.Repository{
			ID:       "repo-" + string(rune(i)),
			Service:  service,
			Owner:    owner,
			Name:     name,
			HTTPSURL: "https://" + service + ".com/" + owner + "/" + name,
		}
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		git.DetectMirrors(ctx, repos)
	}
}
