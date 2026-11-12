/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package prefix_lists

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bpg/terraform-provider-proxmox/proxmox/api"
)

// GetPrefixList retrieves a single SDN prefix list by ID.
func (c *Client) GetPrefixList(ctx context.Context, id string) (*PrefixListData, error) {
	resBody := &struct {
		Data *PrefixListData `json:"data"`
	}{}

	err := c.DoRequest(ctx, http.MethodGet, c.ExpandPath(id), nil, resBody)
	if err != nil {
		return nil, fmt.Errorf("error reading SDN prefix list %s: %w", id, err)
	}

	if resBody.Data == nil {
		return nil, api.ErrNoDataObjectInResponse
	}

	return resBody.Data, nil
}

// CreatePrefixList creates a new SDN prefix list.
func (c *Client) CreatePrefixList(ctx context.Context, data *PrefixList) error {
	err := c.DoRequest(ctx, http.MethodPost, c.basePath(), data, nil)
	if err != nil {
		return fmt.Errorf("error creating SDN prefix list %s: %w", data.ID, err)
	}

	return nil
}

// UpdatePrefixList updates an existing SDN prefix list.
func (c *Client) UpdatePrefixList(ctx context.Context, id string, data *PrefixListUpdate) error {
	err := c.DoRequest(ctx, http.MethodPut, c.ExpandPath(id), data, nil)
	if err != nil {
		return fmt.Errorf("error updating SDN prefix list %s: %w", id, err)
	}

	return nil
}

// DeletePrefixList deletes an SDN prefix list by ID.
func (c *Client) DeletePrefixList(ctx context.Context, id string) error {
	err := c.DoRequest(ctx, http.MethodDelete, c.ExpandPath(id), nil, nil)
	if err != nil {
		return fmt.Errorf("error deleting SDN prefix list %s: %w", id, err)
	}

	return nil
}
