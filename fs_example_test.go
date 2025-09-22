package githubfs_test

import (
	"bufio"
	"fmt"
	"os"

	"github.com/google/go-github/v74/github"

	githubfs "github.com/sagikazarmark/go-github-fs"
)

func Example() {
	client := github.NewClient(nil)

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		client = client.WithAuthToken(token)
	}

	fsys := githubfs.New(githubfs.WithClient(client))

	file, err := fsys.Open("sagikazarmark/locafero/README.md")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()

	fmt.Println(scanner.Text())

	// Output:
	// # Finder library for [Afero](https://github.com/spf13/afero)
}
