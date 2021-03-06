// Author hoenig

package vaultapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

// Auth provides a way to manage what may be
// authenticated to vault.
//
// For now, this API
// supports only the token authentication
// mechanism that is built into vault. Support
// for additional types of authentication may
// be added in future releases.
//
// More information about managing tokens via
// the auth backend can be found here:
// https://www.vaultproject.io/docs/auth/token.html
type Auth interface {
	CreateToken(opts TokenOptions) (CreatedToken, error)
	LookupToken(id string) (LookedUpToken, error)
	LookupSelfToken() (LookedUpToken, error)
	RenewToken(id string, increment time.Duration) (RenewedToken, error)
	RenewSelfToken(increment time.Duration) (RenewedToken, error)
	ListTokenRoles() ([]string, error)
	CreateTokenRole(data TokenRoleOptions) error
	LookupTokenRole(name string) (LookedUpTokenRole, error)
	DeleteTokenRole(name string) error
}

// TokenOptions are used to define properties
// of a token being created. More information
// about the different options can be found in
// the token documentation at:
// https://www.vaultproject.io/docs/concepts/tokens.html
type TokenOptions struct {
	Policies        []string      `json:"policies,omitempty"`
	NoDefaultPolicy bool          `json:"no_default_policy,omitempty"`
	Orphan          bool          `json:"no_parent,omitempty"`
	Renewable       bool          `json:"renewable,omitempty"`
	DisplayName     string        `json:"display_name,omitempty"`
	MaxUses         int           `json:"num_uses,omitempty"`
	TTL             time.Duration `json:"ttl,omitempty"`
	MaxTTL          time.Duration `json:"explicit_max_ttl,omitempty"`
	Period          time.Duration `json:"period,omitmempty"`
}

type createdToken struct {
	Data CreatedToken `json:"auth"`
}

// A CreatedToken represents information returned from
// vault after creating a token. The ID attribute is
// the token itself; this is the value used to authenticate
// with vault later on.
type CreatedToken struct {
	ID            string            `json:"client_token"`
	Policies      []string          `json:"policies"`
	Metadata      map[string]string `json:"metadata"`
	LeaseDuration int               `json:"lease_duration"`
	Renewable     bool              `json:"renewable"`
}

func (c *client) CreateToken(opts TokenOptions) (CreatedToken, error) {
	bs, err := json.Marshal(opts)
	if err != nil {
		return CreatedToken{}, err
	}
	tokenRequest := string(bs)
	c.opts.Logger.Printf("token create request: %v", tokenRequest)

	var ct createdToken
	if err := c.post("/v1/auth/token/create", string(bs), &ct); err != nil {
		return CreatedToken{}, err
	}

	if ct.Data.ID == "" {
		// most likely parse errors on our part
		return CreatedToken{}, errors.Errorf("create token returned empty id")
	}

	return ct.Data, nil
}

// A LookedUpToken represents information returned from
// vault after making a request for information about
// a particular token.
type LookedUpToken struct {
	ID           string   `json:"id"`
	Accessor     string   `json:"accessor"`
	CreationTime int      `json:"creation_time"`
	CreationTTL  int      `json:"creation_ttl"`
	DisplayName  string   `json:"display_name"`
	MaxTTL       int      `json:"explicit_max_ttl"`
	NumUses      int      `json:"num_uses"`
	Orphan       bool     `json:"orphan"`
	Path         string   `json:"path"`
	Policies     []string `json:"policies"`
	TTL          int      `json:"ttl"`
}

type lookedUpTokenWrapper struct {
	Data LookedUpToken `json:"data"`
}

type lookupToken struct {
	Token string `json:"token"`
}

func (c *client) LookupToken(id string) (LookedUpToken, error) {
	var tok lookedUpTokenWrapper
	bs, err := json.Marshal(lookupToken{Token: id})
	if err != nil {
		return LookedUpToken{}, err
	}

	if err := c.post("/v1/auth/token/lookup", string(bs), &tok); err != nil {
		// do not provide token id anywhere
		return LookedUpToken{}, errors.Wrapf(err, "failed to lookup token")
	}

	return tok.Data, nil
}

func (c *client) LookupSelfToken() (LookedUpToken, error) {
	var tok lookedUpTokenWrapper
	if err := c.get("/v1/auth/token/lookup-self", &tok); err != nil {
		// do not provide token id anywhere
		return LookedUpToken{}, errors.Wrapf(err, "failed to lookup self token")
	}
	return tok.Data, nil
}

// A RenewedToken represents information returned from
// vault after making a request to renew a periodic
// token.
type RenewedToken struct {
	ClientToken   string   `json:"client_token"`
	Accessor      string   `json:"accessor"`
	Policies      []string `json:"policies"`
	LeaseDuration int      `json:"lease_duration"`
	Renewable     bool     `json:"renewable"`
}

type wrappedRenewedToken struct {
	Auth RenewedToken `json:"auth"`
}

func (c *client) RenewToken(id string, increment time.Duration) (RenewedToken, error) {
	var tok wrappedRenewedToken
	bs, err := json.Marshal(lookupToken{Token: id})
	if err != nil {
		return RenewedToken{}, err
	}

	inc := strconv.Itoa(int(increment.Seconds()))
	path := fixup("/v1/auth", "token/renew", [2]string{"increment", inc})

	if err := c.post(path, string(bs), &tok); err != nil {
		return RenewedToken{}, errors.Wrapf(err, "failed to renew token")
	}

	return tok.Auth, nil
}

func (c *client) RenewSelfToken(increment time.Duration) (RenewedToken, error) {
	var tok wrappedRenewedToken

	inc := strconv.Itoa(int(increment.Seconds()))
	path := fixup("/v1/auth", "token/renew-self", [2]string{"increment", inc})

	if err := c.post(path, "", &tok); err != nil {
		return RenewedToken{}, errors.Wrapf(err, "failed to self-renew token")
	}

	return tok.Auth, nil
}

type rolesWrapper struct {
	Data roles `json:"data"`
}

type roles struct {
	Keys []string `json:"keys"`
}

func (c *client) ListTokenRoles() ([]string, error) {
	var rolesWrapper rolesWrapper
	requestPath := "/v1/auth/token/roles"
	if err := c.list(requestPath, &rolesWrapper); err != nil {
		return nil, errors.Wrapf(err, "failed to list token roles at %q", requestPath)
	}
	sort.Strings(rolesWrapper.Data.Keys)
	return rolesWrapper.Data.Keys, nil
}

type TokenRoleOptions struct {
	Name               string   `json:"role_name"`
	AllowedPolicies    string   `json:"allowed_policies"`
	DisallowedPolicies string   `json:"disallowed_policies"`
	Orphan             bool     `json:"orphan"`
	Period             string   `json:"period"`
	Renewable          bool     `json:"renewable"`
	ExplicitMaxTTL     int      `json:"explicit_max_ttl"`
	PathSuffix         string   `json:"path_suffix"`
	BoundCIDRs         []string `json:"bound_cidrs"`
}

func (c *client) CreateTokenRole(roleData TokenRoleOptions) error {
	bs, err := json.Marshal(roleData)
	if err != nil {
		return errors.Wrap(err, "marshalling role data to JSON request body")
	}
	c.opts.Logger.Printf("role-create request: %v", string(bs))

	requestPath := fmt.Sprintf("/v1/auth/token/roles/%s", roleData.Name)
	if err := c.post(requestPath, string(bs), nil); err != nil {
		return errors.Wrapf(err, "creating role at %q", requestPath)
	}

	return nil
}

type lookedUpTokenRoleWrapper struct {
	Data LookedUpTokenRole `json:"data"`
}

type LookedUpTokenRole struct {
	AllowedPolicies    []string `json:"allowed_policies"`
	DisallowedPolicies []string `json:"disallowed_policies"`
	ExplicitMaxTTL     int      `json:"explicit_max_ttl"`
	Name               string   `json:"name"`
	Orphan             bool     `json:"orphan"`
	PathSuffix         string   `json:"path_suffix"`
	Period             int      `json:"period"`
	Renewable          bool     `json:"renewable"`
}

func (c *client) LookupTokenRole(name string) (LookedUpTokenRole, error) {
	var lookedUpTokenRoleWrapper lookedUpTokenRoleWrapper
	requestPath := fmt.Sprintf("/v1/auth/token/roles/%s", name)
	if err := c.get(requestPath, &lookedUpTokenRoleWrapper); err != nil {
		return LookedUpTokenRole{}, errors.Wrapf(err, "failed to look up role")
	}
	return lookedUpTokenRoleWrapper.Data, nil
}

func (c *client) DeleteTokenRole(name string) error {
	requestPath := fmt.Sprintf("/v1/auth/token/roles/%s", name)
	if err := c.delete(requestPath); err != nil {
		return errors.Wrapf(err, "failed to delete role %q", name)
	}
	return nil
}
