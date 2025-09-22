package githubfs

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-github/v74/github"
)

func newOptions(t *testing.T) Option {
	t.Helper()

	client := github.NewClient(nil)

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		client = client.WithAuthToken(token)
	}

	return options{
		WithClient(client),
		// WithContext(context.WithValue(t.Context(), github.SleepUntilPrimaryRateLimitResetWhenRateLimited, true)),
	}
}

func TestFS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	fsys := New(
		newOptions(t),
		WithRepository("sagikazarmark", "locafero"),
	)

	err := fstest.TestFS(fsys, "README.md", "LICENSE", "go.mod")
	if err != nil {
		t.Errorf("fstest.TestFS failed: %v", err)
	}
}

func TestNoOwner(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{"empty path", ""},
		{"current directory", "."},
		{"file path without owner", "README.md"},
		{"nested path without owner", "cmd/kubelet"},
	}

	for _, tc := range testCases {
		t.Run(strings.ReplaceAll(tc.name, " ", "_"), func(t *testing.T) {
			fsys := New(newOptions(t))

			_, err := fsys.Open(tc.path)
			if err == nil {
				t.Errorf("expected error when opening %q with no owner/repo", tc.path)
			}
		})
	}
}

const owner = "sagikazarmark"

func TestOwner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	filesystems := []struct {
		name    string
		factory func(t *testing.T) fs.FS
	}{
		{"default", func(t *testing.T) fs.FS { return New(newOptions(t)) }},
		{"with owner", func(t *testing.T) fs.FS { return New(newOptions(t), WithOwner(owner)) }},
		{"with repository", func(t *testing.T) fs.FS { return New(newOptions(t), WithRepository(owner, "")) }},
		{"sub", func(t *testing.T) fs.FS { return mustFS(t)(fs.Sub(New(newOptions(t)), owner)) }},
	}

	for _, filesystem := range filesystems {
		t.Run(strings.ReplaceAll(filesystem.name, " ", "_"), func(t *testing.T) {
			fsys := filesystem.factory(t)

			path := "."
			if filesystem.name == "default" {
				path = owner
			}

			file, err := fsys.Open(path)
			if err != nil {
				t.Fatalf("failed to open %s: %v", path, err)
			}
			defer file.Close()

			stat, err := file.Stat()
			if err != nil {
				t.Errorf("failed to stat: %v", err)
			}

			if !stat.IsDir() {
				t.Error("expected directory when listing repositories")
			}

			dirFile, ok := file.(fs.ReadDirFile)
			if !ok {
				t.Fatal("expected ReadDirFile interface")
			}

			entries, err := dirFile.ReadDir(5) // Limit to avoid rate limits
			if err != nil {
				t.Errorf("failed to read directory: %v", err)
			}

			if len(entries) == 0 {
				t.Error("expected to find repositories")
			}

			// All entries should be directories (repositories)
			for _, entry := range entries {
				if !entry.IsDir() {
					t.Errorf("expected repository %s to be a directory", entry.Name())
				}
			}
		})
	}
}

const repo = "locafero"

func TestRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	filesystems := []struct {
		name    string
		factory func(t *testing.T, owner, repo string) fs.FS
	}{
		{"default", func(t *testing.T, _, _ string) fs.FS { return New(newOptions(t)) }},
		{"with owner", func(t *testing.T, owner, repo string) fs.FS { return New(newOptions(t), WithOwner(owner)) }},
		{"with repository", func(t *testing.T, owner, repo string) fs.FS { return New(newOptions(t), WithRepository(owner, repo)) }},
		{"sub", func(t *testing.T, owner, repo string) fs.FS {
			return mustFS(t)(fs.Sub(New(newOptions(t)), path.Join(owner, repo)))
		}},
		{"with owner sub", func(t *testing.T, owner, repo string) fs.FS {
			return mustFS(t)(fs.Sub(New(newOptions(t), WithOwner(owner)), repo))
		}},
	}

	_ = filesystems

	t.Run("repo root", func(t *testing.T) {
		for _, filesystem := range filesystems {
			t.Run(strings.ReplaceAll(filesystem.name, " ", "_"), func(t *testing.T) {
				fsys := filesystem.factory(t, owner, repo)

				p := "."
				switch true {
				case filesystem.name == "default":
					p = path.Join(owner, repo)
				case filesystem.name == "with owner":
					p = repo
				}

				file, err := fsys.Open(p)
				if err != nil {
					t.Fatalf("failed to open %s: %v", p, err)
				}
				defer file.Close()

				stat, err := file.Stat()
				if err != nil {
					t.Errorf("failed to stat: %v", err)
				}

				if !stat.IsDir() {
					t.Error("expected directory for repository root")
				}

				dirFile, ok := file.(fs.ReadDirFile)
				if !ok {
					t.Fatal("expected ReadDirFile interface")
				}

				entries, err := dirFile.ReadDir(-1)
				if err != nil {
					t.Errorf("failed to read directory: %v", err)
				}

				if len(entries) == 0 {
					t.Error("expected non-empty repository root")
				}

				// Check for common files in typical repos
				expectedFiles := []string{"README.md", "LICENSE"}
				found := make(map[string]bool)
				for _, entry := range entries {
					found[entry.Name()] = true
				}

				foundAny := false
				for _, expected := range expectedFiles {
					if found[expected] {
						foundAny = true
						break
					}
				}
				if !foundAny {
					t.Error("expected to find at least one common file (README.md or LICENSE)")
				}
			})
		}
	})

	t.Run("subdir", func(t *testing.T) {
		for _, filesystem := range filesystems {
			t.Run(strings.ReplaceAll(filesystem.name, " ", "_"), func(t *testing.T) {
				fsys := filesystem.factory(t, "kubernetes", "kubernetes")

				p := "."
				switch true {
				case filesystem.name == "default":
					p = path.Join(owner, repo)
				case filesystem.name == "with owner":
					p = repo
				}

				p = path.Join(p, "cmd")

				file, err := fsys.Open(p)
				if err != nil {
					t.Fatalf("failed to open %s: %v", p, err)
				}
				defer file.Close()

				stat, err := file.Stat()
				if err != nil {
					t.Errorf("failed to stat: %v", err)
				}

				if !stat.IsDir() {
					t.Error("expected directory for subdirectory")
				}

				dirFile, ok := file.(fs.ReadDirFile)
				if !ok {
					t.Fatal("expected ReadDirFile interface")
				}

				entries, err := dirFile.ReadDir(-1)
				if err != nil {
					t.Errorf("failed to read directory: %v", err)
				}

				if len(entries) == 0 {
					t.Error("expected non-empty subdirectory")
				}

				// Check for expected subdirectories
				found := make(map[string]bool)
				for _, entry := range entries {
					found[entry.Name()] = true
				}

				expected := []string{"kubelet", "kube-proxy"}

				foundAny := false
				for _, expected := range expected {
					if found[expected] {
						foundAny = true
						break
					}
				}
				if !foundAny {
					t.Errorf("expected to find at least one of %v", expected)
				}
			})
		}
	})
}

func TestFileOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	fsys := New(
		newOptions(t),
		WithRepository("kubernetes", "kubernetes"),
	)

	t.Run("read file content", func(t *testing.T) {
		file, err := fsys.Open("README.md")
		if err != nil {
			t.Fatalf("failed to open README.md: %v", err)
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			t.Errorf("failed to read file: %v", err)
		}

		if len(content) == 0 {
			t.Error("expected non-empty content")
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "Kubernetes") {
			t.Error("expected content to contain 'Kubernetes'")
		}
	})

	t.Run("file info", func(t *testing.T) {
		file, err := fsys.Open("README.md")
		if err != nil {
			t.Fatalf("failed to open README.md: %v", err)
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			t.Errorf("failed to get file info: %v", err)
		}

		if info.Name() != "README.md" {
			t.Errorf("expected name README.md, got %s", info.Name())
		}

		if info.IsDir() {
			t.Error("README.md should not be a directory")
		}

		if info.Size() <= 0 {
			t.Error("README.md should have positive size")
		}

		if info.Mode() == 0 {
			t.Error("expected non-zero mode")
		}
	})
}

func TestDirectoryOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	fsys := New(
		newOptions(t),
		WithRepository("kubernetes", "kubernetes"),
	)

	t.Run("read directory", func(t *testing.T) {
		file, err := fsys.Open(".")
		if err != nil {
			t.Fatalf("failed to open root directory: %v", err)
		}
		defer file.Close()

		dirFile, ok := file.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected ReadDirFile interface")
		}

		entries, err := dirFile.ReadDir(-1)
		if err != nil {
			t.Errorf("failed to read directory: %v", err)
		}

		if len(entries) == 0 {
			t.Error("expected non-empty directory")
		}

		// Test partial reading
		entries2, err := dirFile.ReadDir(3)
		if err != nil {
			t.Errorf("failed to read directory with limit: %v", err)
		}

		if len(entries2) != 3 {
			t.Errorf("expected 3 entries, got %d", len(entries2))
		}
	})

	t.Run("dir entry info", func(t *testing.T) {
		file, err := fsys.Open(".")
		if err != nil {
			t.Fatalf("failed to open root directory: %v", err)
		}
		defer file.Close()

		dirFile := file.(fs.ReadDirFile)
		entries, err := dirFile.ReadDir(-1)
		if err != nil {
			t.Errorf("failed to read directory: %v", err)
		}

		for _, entry := range entries {
			if entry.Name() == "" {
				t.Error("entry should have non-empty name")
			}

			info, err := entry.Info()
			if err != nil {
				t.Errorf("failed to get info for entry %s: %v", entry.Name(), err)
			}

			if info.Name() != entry.Name() {
				t.Errorf("name mismatch: entry=%s, info=%s", entry.Name(), info.Name())
			}

			if info.IsDir() != entry.IsDir() {
				t.Errorf("IsDir mismatch for %s: entry=%v, info=%v", entry.Name(), entry.IsDir(), info.IsDir())
			}

			if entry.Type() != info.Mode()&fs.ModeType {
				t.Errorf("Type mismatch for %s: entry=%v, info=%v", entry.Name(), entry.Type(), info.Mode()&fs.ModeType)
			}
		}
	})
}

func TestFilesystemTraversal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	fsys := New(
		newOptions(t),
		WithRepository("kubernetes", "kubernetes"),
	)

	t.Run("walk directory", func(t *testing.T) {
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Only walk top-level entries to avoid deep traversal and rate limits
			if path != "." && d.IsDir() && !strings.Contains(path, "/") {
				return fs.SkipDir
			}

			if d.Name() == "" {
				t.Errorf("empty name for path %s", path)
			}

			return nil
		})

		if err != nil {
			t.Errorf("WalkDir failed: %v", err)
		}
	})

	t.Run("glob patterns", func(t *testing.T) {
		matches, err := fs.Glob(fsys, "*.md")
		if err != nil {
			t.Errorf("Glob failed: %v", err)
		}

		if len(matches) == 0 {
			t.Error("expected to find .md files")
		}

		// Should find README.md
		found := false
		for _, match := range matches {
			if filepath.Base(match) == "README.md" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find README.md in glob results")
		}
	})

	t.Run("valid path checks", func(t *testing.T) {
		// Test that we can open known files
		testFiles := []string{"README.md", "LICENSE", "go.mod"}
		for _, file := range testFiles {
			f, err := fsys.Open(file)
			if err != nil {
				t.Errorf("failed to open %s: %v", file, err)
				continue
			}
			f.Close()
		}
	})
}

func mustFS(t *testing.T) func(fsys fs.FS, err error) fs.FS {
	return func(fsys fs.FS, err error) fs.FS {
		if err != nil {
			t.Fatal(err)
		}
		return fsys
	}
}
