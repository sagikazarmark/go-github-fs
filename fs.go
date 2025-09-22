// Package githubfs provides an fs.FS implementation for GitHub repositories using the GitHub API.
package githubfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/go-github/v74/github"
)

// fsys implements fs.FS for GitHub repositories.
type fsys struct {
	ref ref

	ctx    context.Context
	ctxFn  func(context.Context) context.Context
	client *github.Client
}

// New creates a new GitHub filesystem for the specified repository.
func New(opts ...Option) fs.FS {
	f := &fsys{}

	for _, opt := range opts {
		opt.apply(f)
	}

	if f.ctx == nil {
		f.ctx = context.Background()
	}

	if f.ctxFn == nil {
		f.ctxFn = func(ctx context.Context) context.Context {
			return ctx
		}
	}

	if f.client == nil {
		f.client = github.NewClient(nil)
	}

	return f
}

// clone creates a copy of the filesystem.
func (f *fsys) clone(r ref) *fsys {
	return &fsys{
		ref:    r,
		ctx:    f.ctx,
		ctxFn:  f.ctxFn,
		client: f.client,
	}
}

// Open implements the [fs.FS] interface.
func (f *fsys) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	ref := f.ref.join(name)

	if err := ref.validate("open"); err != nil {
		return nil, err
	}

	if ref.repo == "" {
		return f.listRepositories(ref.owner)
	}

	return f.getRepoContent(ref)
}

// listRepositories lists repositories for a given owner
func (f *fsys) listRepositories(owner string) (fs.File, error) {
	opts := &github.RepositoryListByUserOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepos []*github.Repository
	for {
		repos, resp, err := f.client.Repositories.ListByUser(f.ctxFn(f.ctx), owner, opts)
		if err := handleErr(err, "open", "/"+owner); err != nil {
			return nil, err
		}

		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	entries := make([]*dirEntry, len(allRepos))
	for i, repo := range allRepos {
		entries[i] = &dirEntry{
			name:  repo.GetName(),
			isDir: true,
			size:  0,
		}
	}

	return &dir{
		name:    owner,
		entries: entries,
	}, nil
}

// getRepoContent gets content from a specific repository
func (f *fsys) getRepoContent(r ref) (fs.File, error) {
	fileContent, dirContent, _, err := f.client.Repositories.GetContents(f.ctxFn(f.ctx), r.owner, r.repo, r.path, &github.RepositoryContentGetOptions{})
	if err := handleErr(err, "open", r.string()); err != nil {
		return nil, err
	}

	if fileContent != nil {
		content, err := fileContent.GetContent()
		if err != nil {
			return nil, err
		}

		return &file{
			name:    fileContent.GetName(),
			size:    int64(fileContent.GetSize()),
			content: io.NopCloser(strings.NewReader(content)),
		}, nil
	}

	if dirContent != nil {
		entries := make([]*dirEntry, len(dirContent))
		for i, content := range dirContent {
			entries[i] = &dirEntry{
				name:  content.GetName(),
				isDir: content.GetType() == "dir",
				size:  int64(content.GetSize()),
			}
		}

		return &dir{
			name:    path.Base(r.string()),
			entries: entries,
		}, nil
	}

	return nil, errors.New("invalid response: no file or directory returned")
}

// Sub implements the [fs.SubFS] interface.
func (f *fsys) Sub(dir string) (fs.FS, error) {
	if !fs.ValidPath(dir) {
		return nil, &fs.PathError{Op: "sub", Path: dir, Err: fs.ErrInvalid}
	}

	return f.clone(f.ref.join(dir)), nil
}

var (
	_ fs.FS    = (*fsys)(nil)
	_ fs.SubFS = (*fsys)(nil)
	_ fs.File  = (*file)(nil)
)

type file struct {
	name    string
	size    int64
	content io.ReadCloser
}

func (f *file) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:  f.name,
		size:  f.size,
		isDir: false,
	}, nil
}

func (f *file) Read(p []byte) (int, error) {
	return f.content.Read(p)
}

func (f *file) Close() error {
	return f.content.Close()
}

var _ fs.ReadDirFile = (*dir)(nil)

type dir struct {
	name    string
	entries []*dirEntry
	offset  int // tracks the current reading position
}

func (d *dir) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:  d.name,
		isDir: true,
	}, nil
}

func (d *dir) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (d *dir) Close() error {
	return nil
}

func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		// Return all remaining entries from current offset
		remaining := len(d.entries) - d.offset
		if remaining == 0 {
			return []fs.DirEntry{}, nil
		}

		entries := make([]fs.DirEntry, remaining)
		for i := range remaining {
			entries[i] = d.entries[d.offset+i]
		}
		d.offset = len(d.entries) // mark as fully read
		return entries, nil
	}

	// n > 0: return at most n entries
	remaining := len(d.entries) - d.offset
	if remaining == 0 {
		// At end of directory, must return io.EOF
		return []fs.DirEntry{}, io.EOF
	}

	// Determine how many entries to return
	count := min(n, remaining)

	entries := make([]fs.DirEntry, count)
	for i := range count {
		entries[i] = d.entries[d.offset+i]
	}

	d.offset += count

	// If we've read all entries and requested more than available, return io.EOF
	if d.offset >= len(d.entries) && n > count {
		return entries, io.EOF
	}

	return entries, nil
}

var _ fs.FileInfo = (*fileInfo)(nil)

type fileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (fi *fileInfo) Name() string {
	return fi.name
}

func (fi *fileInfo) Size() int64 {
	return fi.size
}

func (fi *fileInfo) Mode() fs.FileMode {
	if fi.isDir {
		return fs.ModeDir | 0o755
	}

	return 0o644
}

func (fi *fileInfo) ModTime() time.Time {
	return time.Time{}
}

func (fi *fileInfo) IsDir() bool {
	return fi.isDir
}

func (fi *fileInfo) Sys() any {
	return nil
}

var _ fs.DirEntry = (*dirEntry)(nil)

type dirEntry struct {
	name  string
	isDir bool
	size  int64
}

func (e *dirEntry) Name() string {
	return e.name
}

func (e *dirEntry) IsDir() bool {
	return e.isDir
}

func (e *dirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}

func (e *dirEntry) Info() (fs.FileInfo, error) {
	return &fileInfo{
		name:  e.name,
		size:  e.size,
		isDir: e.isDir,
	}, nil
}

type ref struct {
	owner string
	repo  string
	path  string
}

func (r ref) join(name string) ref {
	if r.owner != "" && r.repo != "" {
		r.path = path.Join(r.path, name)

		return r
	}

	segments := strings.Split(strings.Trim(path.Clean(name), "/"), "/")
	if name == "" || name == "." {
		segments = nil
	}

	var i int

	if r.owner == "" && len(segments) > i {
		r.owner = segments[i]
		i++
	}

	if r.repo == "" && len(segments) > i {
		r.repo = segments[i]
		i++
	}

	if len(segments) > i {
		r.path = path.Join(r.path, path.Join(segments[i:]...))
	}

	return r
}

func (r ref) validate(op string) error {
	if (r.owner == "" && r.repo != "") || ((r.owner == "" || r.repo == "") && r.path != "") {
		panic("invalid ref: this should never happen")
	}

	if r.owner == "" {
		return &fs.PathError{Op: op, Path: "", Err: errors.New("owner is missing")}
	}

	if r.path != "" && !fs.ValidPath(r.path) {
		return &fs.PathError{Op: op, Path: r.path, Err: fs.ErrInvalid}
	}

	return nil
}

func (r ref) string() string {
	return path.Join("/", r.owner, r.repo, r.path)
}

func handleErr(err error, op string, path string) error {
	if gherr := (*github.ErrorResponse)(nil); errors.As(err, &gherr) {
		switch gherr.Response.StatusCode {
		case http.StatusNotFound:
			return &fs.PathError{Op: op, Path: path, Err: fs.ErrNotExist}
		case http.StatusForbidden, http.StatusUnauthorized:
			return &fs.PathError{Op: op, Path: path, Err: fs.ErrPermission}
		}
		return err
	} else if err != nil {
		return err
	}

	return nil
}
