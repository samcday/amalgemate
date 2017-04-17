package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/samcday/rmarsh"
)

var (
	reposFlag  repos
	portFlag   int
	listenFlag string
)

var (
	gemDirLock sync.RWMutex
	gemDir     map[string]*url.URL
)

func init() {
	flag.IntVar(&portFlag, "port", 8080, "Specify port to listen on (8080)")
	flag.StringVar(&listenFlag, "addr", "127.0.0.1", "Address to bind server to (127.0.0.1)")
	flag.Var(&reposFlag, "repo", "URL of upstream RubyGems repositories. Specify one or more in order of priority.")

	gemDir = make(map[string]*url.URL)
}

type dependencyItem map[rmarsh.Symbol]interface{}

func depQuery(gems []string) ([]gemInfo, error) {
	var all [][]gemInfo

	for _, repo := range reposFlag {
		deps, err := loadDependencies(gems, repo)
		if err != nil {
			return nil, err
		}
		all = append(all, deps)
	}

	deps := mergeDependencies(all)

	updateGemDir(deps)

	return deps, nil
}

func updateGemDir(deps []gemInfo) {
	gemDirLock.Lock()
	defer gemDirLock.Unlock()

	for _, dep := range deps {
		suffix := ""
		if dep.platform != "ruby" {
			suffix = fmt.Sprintf("-%s", dep.platform)
		}

		gemDir[fmt.Sprintf("%s-%s%s", dep.name, dep.version, suffix)] = dep.repo
	}
}

func main() {
	flag.Parse()

	if len(reposFlag) == 0 {
		fmt.Println("Need at least one repository specified!")
		flag.Usage()
		os.Exit(1)
	}

	http.HandleFunc("/api/v1/dependencies", func(w http.ResponseWriter, r *http.Request) {
		gems := r.URL.Query().Get("gems")
		if gems == "" {
			return
		}

		result, err := depQuery(strings.Split(gems, ","))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var resp []dependencyItem
		for _, item := range result {
			var deps [][]string
			for _, d := range item.deps {
				deps = append(deps, []string{d.name, d.ver})
			}
			resp = append(resp, dependencyItem{
				rmarsh.Symbol("name"):         item.name,
				rmarsh.Symbol("number"):       item.version,
				rmarsh.Symbol("platform"):     item.platform,
				rmarsh.Symbol("dependencies"): deps,
			})
		}

		rmarsh.NewEncoder(w).Encode(resp)
	})

	http.HandleFunc("/gems/", func(w http.ResponseWriter, r *http.Request) {
		gem := strings.TrimPrefix(r.URL.Path, "/gems/")

		gemDirLock.RLock()
		repo, found := gemDir[strings.TrimSuffix(gem, ".gem")]
		gemDirLock.RUnlock()

		if !found {
			w.WriteHeader(404)
		}

		fmt.Printf("Found %s in repo %s\n", gem, repo)

		http.Redirect(w, r, fmt.Sprintf("%sgems/%s", repo, gem), http.StatusMovedPermanently)
	})

	addr := fmt.Sprintf("%s:%d", listenFlag, portFlag)
	fmt.Println("Listening on", addr)
	http.ListenAndServe(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL)
		http.DefaultServeMux.ServeHTTP(w, r)
	}))
}

type gemInfo struct {
	repo                    *url.URL
	name, version, platform string
	deps                    []*gemDepInfo
}

func (gi *gemInfo) fromMarshal(v interface{}) error {
	item, ok := v.(map[interface{}]interface{})
	if !ok {
		return fmt.Errorf("Unexpected type %T in dependencies data", v)
	}
	for k, v := range item {
		sym, ok := k.(rmarsh.Symbol)
		if !ok {
			return fmt.Errorf("Unexpected type %T in dependencies data", k)
		}

		if sym == rmarsh.Symbol("name") {
			gi.name = v.(string)
		} else if sym == rmarsh.Symbol("number") {
			gi.version = v.(string)
		} else if sym == rmarsh.Symbol("platform") {
			gi.platform = v.(string)
		} else if sym == rmarsh.Symbol("dependencies") {
			for _, dep := range v.([]interface{}) {
				gpi := new(gemDepInfo)
				gpi.fromMarshal(dep)
				gi.deps = append(gi.deps, gpi)
			}
		}
	}

	return nil
}

type gemDepInfo struct {
	name, ver string
}

func (gpi *gemDepInfo) fromMarshal(v interface{}) error {
	item, ok := v.([]interface{})
	if !ok {
		return fmt.Errorf("Unexpected type %T in dependencies data", v)
	}

	for i, x := range item {
		if i == 0 {
			gpi.name = x.(string)
		} else if i == 1 {
			gpi.ver = x.(string)
		}
	}

	return nil
}

func loadDependencies(deps []string, repo *url.URL) ([]gemInfo, error) {
	u := repo.ResolveReference(&url.URL{Path: ""})
	u.Query().Add("gems", strings.Join(deps, ","))
	res, err := http.Get(fmt.Sprintf("%s%s?gems=%s", repo, "api/v1/dependencies", url.QueryEscape(strings.Join(deps, ","))))
	if err != nil {
		return nil, err
	}

	r := rmarsh.NewDecoder(res.Body)
	raw, err := r.Decode()
	if err != nil {
		return nil, err
	}

	var results []gemInfo

	if raw == nil {
		return results, nil
	}

	rawl, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("Unexpected type %T in dependencies data", raw)
	}

	for _, x := range rawl {
		info := new(gemInfo)
		info.repo = repo
		if err := info.fromMarshal(x); err != nil {
			return nil, err
		}
		results = append(results, *info)
	}

	return results, nil
}

// Merges together multiple dep lists in priority order.
func mergeDependencies(deps [][]gemInfo) []gemInfo {
	var merged []gemInfo
	seen := make(map[string]bool)

	for _, rdeps := range deps {
		for _, dep := range rdeps {
			id := fmt.Sprintf("%s-%s-%s", dep.name, dep.version, dep.platform)
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = true
			merged = append(merged, dep)
		}
	}

	return merged
}

type repos []*url.URL

func (s *repos) String() string {
	return fmt.Sprint(*s)
}

func (s *repos) Set(v string) error {
	u, err := url.Parse(v)
	if err != nil {
		return err
	}

	*s = append(*s, u)
	return nil
}
