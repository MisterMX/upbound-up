// Copyright 2023 Upbound Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloud

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd/api"

	sdkerrs "github.com/upbound/up-sdk-go/errors"
	"github.com/upbound/up-sdk-go/service/common"
	"github.com/upbound/up-sdk-go/service/configurations"
	"github.com/upbound/up-sdk-go/service/controlplanes"
	"github.com/upbound/up/internal/controlplane"
	"github.com/upbound/up/internal/kube"
)

const (
	maxItems = 100
)

type ctpClient interface {
	Create(ctx context.Context, account string, params *controlplanes.ControlPlaneCreateParameters) (*controlplanes.ControlPlaneResponse, error)
	Delete(ctx context.Context, account, name string) error
	Get(ctx context.Context, account, name string) (*controlplanes.ControlPlaneResponse, error)
	List(ctx context.Context, account string, opts ...common.ListOption) (*controlplanes.ControlPlaneListResponse, error)
}

type cfgGetter interface {
	Get(ctx context.Context, account, name string) (*configurations.ConfigurationResponse, error)
}

type Option func(*Client)

func WithToken(t string) Option {
	return func(c *Client) {
		c.token = t
	}
}

func WithProxyEndpoint(p *url.URL) Option {
	return func(c *Client) {
		c.proxy = p
	}
}

// Client is the client used for interacting with the ControlPlanes API in
// Upbound Cloud.
type Client struct {
	ctp ctpClient
	cfg cfgGetter

	// Upbound Account
	account string
	// Cloud PAT for Control Plane Kubeconfig.
	token string
	// Proxy Endppint corresponding to Upbound Cloud's Proxy.
	proxy *url.URL
}

// New instantiates a new Client.
func New(ctp ctpClient, cfg cfgGetter, account string, opts ...Option) *Client {
	c := &Client{
		ctp:     ctp,
		cfg:     cfg,
		account: account,
	}

	for _, o := range opts {
		o(c)
	}
	return c
}

// Get the ControlPlane corresponding to the given ControlPlane name.
func (c *Client) Get(ctx context.Context, ctp types.NamespacedName) (*controlplane.Response, error) {
	if ctp.Namespace != "" {
		return nil, errors.New("namespace is not supported for Upbound Cloud control planes")
	}
	resp, err := c.ctp.Get(ctx, c.account, ctp.Name)

	if sdkerrs.IsNotFound(err) {
		return nil, controlplane.NewNotFound(err)
	}

	if err != nil {
		return nil, err
	}

	return convert(resp), nil
}

// List all ControlPlanes within the Upbound Cloud account.
func (c *Client) List(ctx context.Context, namespace string) ([]*controlplane.Response, error) {
	if namespace != "" {
		return nil, errors.New("namespace is not supported for Upbound Cloud control planes")
	}
	l, err := c.ctp.List(ctx, c.account, common.WithSize(maxItems))
	if err != nil {
		return nil, err
	}
	resps := []*controlplane.Response{}
	for _, r := range l.ControlPlanes {
		cp := r
		resps = append(resps, convert(&cp))
	}
	return resps, nil
}

// Create a new ControlPlane with the given name and the supplied Options.
func (c *Client) Create(ctx context.Context, ctp types.NamespacedName, opts controlplane.Options) (*controlplane.Response, error) {
	if ctp.Namespace != "" {
		return nil, errors.New("namespace is not supported for Upbound Cloud control planes")
	}
	params := &controlplanes.ControlPlaneCreateParameters{
		Name:        ctp.Name,
		Description: opts.Description,
	}
	if opts.ConfigurationName != nil {
		// Get the UUID from the Configuration name, if it exists.
		cfg, err := c.cfg.Get(ctx, c.account, *opts.ConfigurationName)
		if err != nil {
			return nil, err
		}
		params.ConfigurationID = &cfg.ID
	}

	resp, err := c.ctp.Create(ctx, c.account, params)
	if err != nil {
		return nil, err
	}

	return convert(resp), nil
}

// Delete the ControlPlane corresponding to the given ControlPlane name.
func (c *Client) Delete(ctx context.Context, ctp types.NamespacedName) error {
	if ctp.Namespace != "" {
		return errors.New("namespace is not supported for Upbound Cloud control planes")
	}
	err := c.ctp.Delete(ctx, c.account, ctp.Name)
	if sdkerrs.IsNotFound(err) {
		return controlplane.NewNotFound(err)
	}
	return err
}

// GetKubeConfig for the given Control Plane.
func (c *Client) GetKubeConfig(ctx context.Context, ctp types.NamespacedName) (*api.Config, error) {
	return kube.BuildControlPlaneKubeconfig(
		c.proxy,
		path.Join(c.account, ctp.Name),
		c.token,
		false,
	), nil
}

func convert(ctp *controlplanes.ControlPlaneResponse) *controlplane.Response {
	var cfgName string
	var cfgStatus controlplanes.ConfigurationStatus
	if ctp.ControlPlane.Configuration != nil {
		cfgName = *ctp.ControlPlane.Configuration.Name
		cfgStatus = ctp.ControlPlane.Configuration.Status
	}

	var age *time.Duration
	if ctp.ControlPlane.CreatedAt != nil {
		d := time.Since(*ctp.ControlPlane.CreatedAt)
		age = &d
	}

	return &controlplane.Response{
		ID:      ctp.ControlPlane.ID.String(),
		Name:    ctp.ControlPlane.Name,
		Synced:  toBool(true),
		Ready:   toBool(ctp.Status == controlplanes.StatusReady),
		Message: toMessage(ctp.Status),
		Cfg:     cfgName,
		Updated: formatStatus(cfgStatus),
		Age:     age,
	}
}

func formatStatus(status controlplanes.ConfigurationStatus) string {
	switch status { // nolint: exhaustive
	case "":
		return ""
	case controlplanes.ConfigurationReady:
		return "True"
	default:
		return strings.ToUpper(string(status)[:1]) + string(status)[1:]
	}
}

func toMessage(status controlplanes.Status) string {
	switch status { // nolint: exhaustive
	case controlplanes.StatusProvisioning:
		return "Controlplane is being created"
	case controlplanes.StatusUpdating:
		return "Controlplane is being updated"
	case controlplanes.StatusDeleting:
		return "Controlplane is being deleted"
	default:
		return ""
	}
}

func toBool(b bool) string {
	if b {
		return "True"
	}
	return "False"
}
