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
	"fmt"

	"code.gitea.io/sdk/gitea"
	"github.com/fluxcd/go-git-providers/gitprovider"
)

// giteaClientImpl is a wrapper around *gitea.Client, which implements higher-level methods,
// operating on the gitea structs. Pagination is implemented for all List* methods,
// all returned objects are validated, and HTTP errors are handled/wrapped using handleHTTPError.
// This interface is also fakeable, in order to unit-test the client.
type giteaClient interface {
	// Client returns the underlying *gitea.Client
	Client() *gitea.Client

	// GetOrg is a wrapper for "GET /orgs/{org}".
	// This function HTTP error wrapping, and validates the server result.
	GetOrg(orgName string) (*gitea.Organization, error)
	// ListOrgs is a wrapper for "GET /user/orgs".
	ListOrgs() ([]*gitea.Organization, error)

	// ListOrgTeamMembers is a wrapper for "GET /orgs/{org}/teams" then "GET /teams/{team}/members".
	ListOrgTeamMembers(orgName, teamName string) ([]*gitea.User, error)
	// ListOrgTeams is a wrapper for "GET /orgs/{org}/teams".
	ListOrgTeams(orgName string) ([]*gitea.Team, error)

	// GetRepo is a wrapper for "GET /repos/{owner}/{repo}".
	// This function handles HTTP error wrapping, and validates the server result.
	GetRepo(owner, repo string) (*gitea.Repository, error)
	// ListOrgRepos is a wrapper for "GET /orgs/{org}/repos".
	ListOrgRepos(org string) ([]*gitea.Repository, error)
	// ListUserRepos is a wrapper for "GET /users/{username}/repos".
	ListUserRepos(username string) ([]*gitea.Repository, error)
	// CreateRepo is a wrapper for "POST /user/repos" (if orgName == "")
	// or "POST /orgs/{org}/repos" (if orgName != "").
	// This function handles HTTP error wrapping, and validates the server result.
	CreateRepo(orgName string, req *gitea.CreateRepoOption) (*gitea.Repository, error)
	// UpdateRepo is a wrapper for "PATCH /repos/{owner}/{repo}".
	// This function handles HTTP error wrapping, and validates the server result.
	UpdateRepo(owner, repo string, req *gitea.EditRepoOption) (*gitea.Repository, error)
	// DeleteRepo is a wrapper for "DELETE /repos/{owner}/{repo}".
	// This function handles HTTP error wrapping.
	// DANGEROUS COMMAND: In order to use this, you must set destructiveActions to true.
	DeleteRepo(owner, repo string) error

	// ListKeys is a wrapper for "GET /repos/{owner}/{repo}/keys".
	// This function handles pagination, HTTP error wrapping, and validates the server result.
	ListKeys(owner, repo string) ([]*gitea.DeployKey, error)
	// ListCommitsPage is a wrapper for "GET /repos/{owner}/{repo}/git/commits".
	// This function handles pagination, HTTP error wrapping.
	ListCommitsPage(owner, repo, branch string, perPage int, page int) ([]*gitea.Commit, error)
	// CreateKey is a wrapper for "POST /repos/{owner}/{repo}/keys".
	// This function handles HTTP error wrapping, and validates the server result.
	CreateKey(owner, repo string, req *gitea.DeployKey) (*gitea.DeployKey, error)
	// DeleteKey is a wrapper for "DELETE /repos/{owner}/{repo}/keys/{key_id}".
	// This function handles HTTP error wrapping.
	DeleteKey(owner, repo string, id int64) error

	// GetTeamPermissions is a wrapper for "GET /repos/{owner}/{repo}/teams/{team_slug}
	// This function handles HTTP error wrapping, and validates the server result.
	GetTeamPermissions(orgName, repo, teamName string) (*gitea.AccessMode, error)
	// GetRepoTeams is a wrapper for "GET /repos/{owner}/{repo}/teams".
	// This function handles pagination, HTTP error wrapping, and validates the server result.
	GetRepoTeams(orgName, repo string) ([]*gitea.Team, error)
	// AddTeam is a wrapper for "PUT /repos/{owner}/{repo}/teams/{team_slug}".
	// This function handles HTTP error wrapping.
	AddTeam(orgName, repo, teamName string, permission gitprovider.RepositoryPermission) error
	// RemoveTeam is a wrapper for "DELETE /repos/{owner}/{repo}/teams/{team_slug}".
	// This function handles HTTP error wrapping.
	RemoveTeam(orgName, repo, teamName string) error
}

type giteaClientImpl struct {
	c                  *gitea.Client
	destructiveActions bool
}

var _ giteaClient = &giteaClientImpl{}

func (c *giteaClientImpl) Client() *gitea.Client {
	return c.c
}

func (c *giteaClientImpl) GetOrg(orgName string) (*gitea.Organization, error) {
	apiObj, res, err := c.c.GetOrg(orgName)
	if err != nil {
		return nil, handleHTTPError(res, err)
	}
	// Validate the API object
	if err := validateOrganizationAPI(apiObj); err != nil {
		return nil, err
	}
	return apiObj, nil
}

func (c *giteaClientImpl) ListOrgs() ([]*gitea.Organization, error) {
	opts := gitea.ListOrgsOptions{}
	apiObjs := []*gitea.Organization{}
	listOpts := &opts.ListOptions

	err := allPages(listOpts, func() (*gitea.Response, error) {
		// GET /user/orgs"
		pageObjs, resp, listErr := c.c.ListMyOrgs(opts)
		if len(pageObjs) > 0 {
			apiObjs = append(apiObjs, pageObjs...)
			return resp, listErr
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	// Validate the API objects
	for _, apiObj := range apiObjs {
		if err := validateOrganizationAPI(apiObj); err != nil {
			return nil, err
		}
	}
	return apiObjs, nil
}

func (c *giteaClientImpl) ListOrgTeamMembers(orgName, teamName string) ([]*gitea.User, error) {
	teams, err := c.ListOrgTeams(orgName)
	if err != nil {
		return nil, err
	}

	for _, team := range teams {
		if team.Name == teamName {
			users, _, err := c.c.ListTeamMembers(team.ID, gitea.ListTeamMembersOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to list team %s members: %w", teamName, err)
			}
			return users, nil
		}
	}
	return nil, gitprovider.ErrNotFound
}

func (c *giteaClientImpl) ListOrgTeams(orgName string) ([]*gitea.Team, error) {
	opts := gitea.ListTeamsOptions{}
	apiObjs := []*gitea.Team{}
	listOpts := &opts.ListOptions

	err := allPages(listOpts, func() (*gitea.Response, error) {
		// GET /orgs/{org}/teams"
		pageObjs, resp, listErr := c.c.ListOrgTeams(orgName, opts)
		if len(pageObjs) > 0 {
			apiObjs = append(apiObjs, pageObjs...)
			return resp, listErr
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return apiObjs, nil
}

func (c *giteaClientImpl) GetRepo(owner, repo string) (*gitea.Repository, error) {
	apiObj, res, err := c.c.GetRepo(owner, repo)
	return validateRepositoryAPIResp(apiObj, res, err)
}

func validateRepositoryAPIResp(apiObj *gitea.Repository, res *gitea.Response, err error) (*gitea.Repository, error) {
	// If the response contained an error, return
	if err != nil {
		return nil, handleHTTPError(res, err)
	}
	// Make sure apiObj is valid
	if err := validateRepositoryAPI(apiObj); err != nil {
		return nil, err
	}
	return apiObj, nil
}

func (c *giteaClientImpl) ListOrgRepos(org string) ([]*gitea.Repository, error) {
	opts := gitea.ListOrgReposOptions{}
	apiObjs := []*gitea.Repository{}
	listOpts := &opts.ListOptions

	err := allPages(listOpts, func() (*gitea.Response, error) {
		// GET /orgs/{org}/repos
		pageObjs, resp, listErr := c.c.ListOrgRepos(org, opts)
		if len(pageObjs) > 0 {
			apiObjs = append(apiObjs, pageObjs...)
			return resp, listErr
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return validateRepositoryObjects(apiObjs)
}

func validateRepositoryObjects(apiObjs []*gitea.Repository) ([]*gitea.Repository, error) {
	for _, apiObj := range apiObjs {
		// Make sure apiObj is valid
		if err := validateRepositoryAPI(apiObj); err != nil {
			return nil, err
		}
	}
	return apiObjs, nil
}

func (c *giteaClientImpl) ListUserRepos(username string) ([]*gitea.Repository, error) {
	opts := gitea.ListReposOptions{}
	apiObjs := []*gitea.Repository{}
	listOpts := &opts.ListOptions

	err := allPages(listOpts, func() (*gitea.Response, error) {
		// GET /users/{username}/repos
		pageObjs, resp, listErr := c.c.ListUserRepos(username, opts)
		if len(pageObjs) > 0 {
			apiObjs = append(apiObjs, pageObjs...)
			return resp, listErr
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return validateRepositoryObjects(apiObjs)
}

func (c *giteaClientImpl) CreateRepo(orgName string, req *gitea.CreateRepoOption) (*gitea.Repository, error) {
	if orgName != "" {
		apiObj, res, err := c.c.CreateOrgRepo(orgName, *req)
		return validateRepositoryAPIResp(apiObj, res, err)
	}
	apiObj, res, err := c.c.CreateRepo(*req)
	return validateRepositoryAPIResp(apiObj, res, err)
}

func (c *giteaClientImpl) UpdateRepo(owner, repo string, req *gitea.EditRepoOption) (*gitea.Repository, error) {
	apiObj, res, err := c.c.EditRepo(owner, repo, *req)
	return validateRepositoryAPIResp(apiObj, res, err)
}

func (c *giteaClientImpl) DeleteRepo(owner, repo string) error {
	// Don't allow deleting repositories if the user didn't explicitly allow dangerous API calls.
	if !c.destructiveActions {
		return fmt.Errorf("cannot delete repository: %w", gitprovider.ErrDestructiveCallDisallowed)
	}
	res, err := c.c.DeleteRepo(owner, repo)
	return handleHTTPError(res, err)
}

func (c *giteaClientImpl) ListKeys(owner, repo string) ([]*gitea.DeployKey, error) {
	opts := gitea.ListDeployKeysOptions{}
	apiObjs := []*gitea.DeployKey{}
	listOpts := &opts.ListOptions

	err := allPages(listOpts, func() (*gitea.Response, error) {
		// GET /repos/{owner}/{repo}/keys"
		pageObjs, resp, listErr := c.c.ListDeployKeys(owner, repo, opts)
		if len(pageObjs) > 0 {
			apiObjs = append(apiObjs, pageObjs...)
			return resp, listErr
		}
		return nil, nil
	})

	if err != nil {
		return nil, err
	}

	for _, apiObj := range apiObjs {
		if err := validateDeployKeyAPI(apiObj); err != nil {
			return nil, err
		}
	}
	return apiObjs, nil
}

func (c *giteaClientImpl) ListCommitsPage(owner, repo, branch string, perPage int, page int) ([]*gitea.Commit, error) {
	opts := gitea.ListCommitOptions{
		ListOptions: gitea.ListOptions{
			PageSize: perPage,
			Page:     page,
		},
		SHA: branch,
	}
	apiObjs, _, listErr := c.c.ListRepoCommits(owner, repo, opts)

	if listErr != nil {
		return nil, listErr
	}
	return apiObjs, nil
}

func (c *giteaClientImpl) CreateKey(owner, repo string, req *gitea.DeployKey) (*gitea.DeployKey, error) {
	opts := gitea.CreateKeyOption{Title: req.Title, Key: req.Key, ReadOnly: req.ReadOnly}
	apiObj, res, err := c.c.CreateDeployKey(owner, repo, opts)
	if err != nil {
		return nil, handleHTTPError(res, err)
	}
	if err := validateDeployKeyAPI(apiObj); err != nil {
		return nil, err
	}
	return apiObj, nil
}

func (c *giteaClientImpl) DeleteKey(owner, repo string, id int64) error {
	res, err := c.c.DeleteDeployKey(owner, repo, id)
	return handleHTTPError(res, err)
}

func (c *giteaClientImpl) GetTeamPermissions(orgName, repo, teamName string) (*gitea.AccessMode, error) {
	apiObj, res, err := c.c.CheckRepoTeam(orgName, repo, teamName)
	if err != nil {
		return nil, handleHTTPError(res, err)
	}

	return &apiObj.Permission, nil
}

func (c *giteaClientImpl) GetRepoTeams(orgName, repo string) ([]*gitea.Team, error) {
	apiObjs, err := c.GetRepoTeams(orgName, repo)
	if err != nil {
		return nil, err
	}
	return apiObjs, nil
}

func (c *giteaClientImpl) AddTeam(orgName, repo, teamName string, permission gitprovider.RepositoryPermission) error {
	res, err := c.c.AddRepoTeam(orgName, repo, teamName)
	return handleHTTPError(res, err)
}

func (c *giteaClientImpl) RemoveTeam(orgName, repo, teamName string) error {
	res, err := c.c.RemoveRepoTeam(orgName, repo, teamName)
	return handleHTTPError(res, err)
}
