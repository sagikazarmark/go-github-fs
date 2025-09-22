package githubfs

import (
	"context"

	"github.com/google/go-github/v74/github"
)

// ClientOption configures the filesystem using the functional options paradigm popularized by Rob Pike and Dave Cheney.
//
// If you're unfamiliar with this style, check out the following articles:
//   - [Self-referential functions and the design of options]
//   - [Functional options for friendly APIs]
//   - [Functional options on steroids]
//
// [Self-referential functions and the design of options]: https://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html and
// [Functional options for friendly APIs]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis.
// [Functional options on steroids]: https://sagikazarmark.com/blog/posts/functional-options-on-steroids/
type Option interface {
	apply(c *fsys)
}

type optionFunc func(*fsys)

func (fn optionFunc) apply(f *fsys) {
	fn(f)
}

type options []Option

func (o options) apply(f *fsys) {
	for _, opt := range o {
		opt.apply(f)
	}
}

// WithOwner configures the owner.
func WithOwner(owner string) Option {
	return optionFunc(func(f *fsys) {
		if owner == "" {
			return
		}

		f.ref.owner = owner
	})
}

// WithRepository configures the repository.
func WithRepository(owner string, repo string) Option {
	return optionFunc(func(f *fsys) {
		if owner != "" {
			f.ref.owner = owner
		}

		if repo != "" {
			f.ref.repo = repo
		}
	})
}

// WithClient configures a [github.Client].
func WithClient(c *github.Client) Option {
	return optionFunc(func(f *fsys) {
		f.client = c
	})
}

// WithContext configures a [context.Context].
func WithContext(ctx context.Context) Option {
	return optionFunc(func(f *fsys) {
		f.ctx = ctx
	})
}

// WithContextFunc configures a function that creates a new context for each request.
func WithContextFunc(fn func(context.Context) context.Context) Option {
	return optionFunc(func(f *fsys) {
		f.ctxFn = fn
	})
}
