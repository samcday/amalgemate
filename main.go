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

func updateGemDir(deps []gemInfo) {
	gemDirLock.Lock()
	defer gemDirLock.Unlock()

	for _, dep := range deps {
		gemDir[dep.ident()] = dep.repo
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

		rmarsh.NewEncoder(w).Encode(result)
	})

	http.HandleFunc("/gems/", func(w http.ResponseWriter, r *http.Request) {
		gem := strings.TrimPrefix(r.URL.Path, "/gems/")

		gemDirLock.RLock()
		repo, found := gemDir[strings.TrimSuffix(gem, ".gem")]
		gemDirLock.RUnlock()

		if !found {
			w.WriteHeader(404)
			return
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
