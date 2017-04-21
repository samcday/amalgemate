package main

// Handle the /api/v1/dependencies API.

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/samcday/rmarsh"
)

type gemInfo struct {
	repo         *url.URL
	Name         string     `rmarsh:"name"`
	Version      string     `rmarsh:"number"`
	Platform     string     `rmarsh:"platform"`
	Dependencies [][]string `rmarsh:"dependencies"`
}

func (g *gemInfo) ident() string {
	suffix := ""
	if g.Platform != "ruby" {
		suffix = fmt.Sprintf("-%s", g.Platform)
	}
	return fmt.Sprintf("%s-%s%s", g.Name, g.Version, suffix)
}

// Queries one or more remote repos for the dependency info on one or more gems.
// Merges the results and returns them.
func depQuery(gems []string) ([]gemInfo, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	all := make([][]gemInfo, len(reposFlag))
	var repoErr error

	for i, repo := range reposFlag {
		wg.Add(1)
		go func(i int) {
			deps, err := loadDependencies(gems, repo)

			mu.Lock()
			if err != nil {
				if repoErr == nil {
					repoErr = err
				}
			} else {
				all[i] = deps
			}
			mu.Unlock()
			wg.Done()
		}(i)
	}

	wg.Wait()

	if repoErr != nil {
		return nil, repoErr
	}

	deps := mergeDependencies(all)
	updateGemDir(deps)
	return deps, nil
}

func loadDependencies(deps []string, repo *url.URL) ([]gemInfo, error) {
	u := repo.ResolveReference(&url.URL{Path: ""})
	u.Query().Add("gems", strings.Join(deps, ","))
	res, err := http.Get(fmt.Sprintf("%s%s?gems=%s", repo, "api/v1/dependencies", url.QueryEscape(strings.Join(deps, ","))))
	if err != nil {
		return nil, err
	}

	r := rmarsh.NewDecoder(res.Body)
	var results []gemInfo
	if err := r.Decode(&results); err != nil {
		return nil, err
	}

	for i := range results {
		results[i].repo = repo
	}

	return results, nil
}

// Merges together multiple dep lists in priority order.
func mergeDependencies(deps [][]gemInfo) []gemInfo {
	var merged []gemInfo
	seen := make(map[string]bool)

	for _, rdeps := range deps {
		for _, dep := range rdeps {
			if _, ok := seen[dep.ident()]; ok {
				continue
			}
			seen[dep.ident()] = true
			merged = append(merged, dep)
		}
	}

	return merged
}
