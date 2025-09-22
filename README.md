# Go GitHub filesystem

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/sagikazarmark/go-github-fs/ci.yaml?style=flat-square)](https://github.com/sagikazarmark/go-github-fs/actions/workflows/ci.yaml)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/mod/github.com/sagikazarmark/go-github-fs)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/sagikazarmark/go-github-fs?style=flat-square&color=61CFDD)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/sagikazarmark/go-github-fs/badge?style=flat-square)](https://deps.dev/go/github.com%252Fsagikazarmark%252Fgo-github-fs)

**Go [fs.FS](https://pkg.go.dev/io/fs#FS) implementation for the GitHub Contents API**

## Installation

```shell
go get github.com/sagikazarmark/go-github-fs
```

## Usage

```go
package main

import (
	"github.com/google/go-github/v74/github"
	"github.com/sagikazarmark/go-github-fs"
)

func main() {
	client := github.NewClient(nil)

	fsys := githubfs.New(githubfs.WithClient(client))

	file, err := fsys.Open("sagikazarmark/go-github-fs/README.md")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	_ = file
}
```

Check out the [documentation](https://pkg.go.dev/mod/github.com/sagikazarmark/go-github-fs) for all options.

## License

The project is licensed under the [MIT License](LICENSE).
