/*
Copyright 2020 The Flux CD contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gitea

import (
	"context"

	"code.gitea.io/sdk/gitea"
	"github.com/fluxcd/go-git-providers/gitprovider"
)

// PullRequestClient implements the gitprovider.PullRequestClient interface.
var _ gitprovider.PullRequestClient = &PullRequestClient{}

// PullRequestClient operates on the pull requests for a specific repository.
type PullRequestClient struct {
	*clientContext
	ref gitprovider.RepositoryRef
}

// List lists all pull requests in the repository
func (c *PullRequestClient) List(ctx context.Context) ([]gitprovider.PullRequest, error) {
	opts := gitea.ListPullRequestsOptions{}
	prs, _, err := c.c.Client().ListRepoPullRequests(c.ref.GetIdentity(), c.ref.GetRepository(), opts)
	// prs, _, err := c.c.Client().PullRequests.List(ctx, c.ref.GetIdentity(), c.ref.GetRepository(), nil)
	if err != nil {
		return nil, err
	}

	requests := make([]gitprovider.PullRequest, len(prs))

	for idx, pr := range prs {
		requests[idx] = newPullRequest(c.clientContext, pr)
	}

	return requests, nil
}

// Create creates a pull request with the given specifications.
func (c *PullRequestClient) Create(ctx context.Context, title, branch, baseBranch, description string) (gitprovider.PullRequest, error) {

	prOpts := gitea.CreatePullRequestOption{
		Title: title,
	}
	pr, _, err := c.c.Client().CreatePullRequest(c.ref.GetIdentity(), c.ref.GetRepository(), prOpts)

	if err != nil {
		return nil, err
	}

	return newPullRequest(c.clientContext, pr), nil
}

// Get retrieves an existing pull request by number
func (c *PullRequestClient) Get(ctx context.Context, number int) (gitprovider.PullRequest, error) {
	pr, _, err := c.c.Client().GetPullRequest(c.ref.GetIdentity(), c.ref.GetRepository(), int64(number))
	// pr, _, err := c.c.Client().PullRequests.Get()
	if err != nil {
		return nil, err
	}

	return newPullRequest(c.clientContext, pr), nil
}

// Merge merges a pull request with the given specifications.
func (c *PullRequestClient) Merge(ctx context.Context, number int, mergeMethod gitprovider.MergeMethod, message string) error {

	mergeOpts := gitea.MergePullRequestOption{
		Style:   gitea.MergeStyle(mergeMethod),
		Message: message,
	}

	_, _, err := c.c.Client().MergePullRequest(c.ref.GetIdentity(), c.ref.GetRepository(), int64(number), mergeOpts)

	if err != nil {
		return err
	}

	return nil
}