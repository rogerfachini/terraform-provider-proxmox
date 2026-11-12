/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package prefix_lists

import (
	"fmt"
	"strconv"
	"strings"
)

/*
PrefixList used to represent an SDN prefix list in the API.

Based on docs:
  - https://pve.proxmox.com/pve-docs/api-viewer/index.html#/cluster/sdn/prefix-lists

Entries are PVE property strings, e.g. "action=permit,prefix=10.0.0.0/16,le=32,seq=5".
*/
type PrefixList struct {
	ID      string   `json:"id"                url:"id"`
	Entries []string `json:"entries,omitempty" url:"entries,omitempty"`
}

// PrefixListData is the response payload of a prefix list read.
type PrefixListData struct {
	PrefixList

	Digest *string `json:"digest,omitempty"`
}

// PrefixListUpdate is the request body of a prefix list update. The ID is carried in the URL path.
type PrefixListUpdate struct {
	Entries []string `url:"entries,omitempty"`
	Delete  []string `url:"delete,omitempty"`
}

// Entry is a single prefix list entry.
type Entry struct {
	Seq    *int64
	Action string
	Prefix string
	GE     *int64
	LE     *int64
}

// String renders the entry as a PVE property string.
func (e Entry) String() string {
	parts := []string{"action=" + e.Action, "prefix=" + e.Prefix}

	if e.GE != nil {
		parts = append(parts, "ge="+strconv.FormatInt(*e.GE, 10))
	}

	if e.LE != nil {
		parts = append(parts, "le="+strconv.FormatInt(*e.LE, 10))
	}

	if e.Seq != nil {
		parts = append(parts, "seq="+strconv.FormatInt(*e.Seq, 10))
	}

	return strings.Join(parts, ",")
}

// ParseEntry parses a PVE property string into an entry.
func ParseEntry(s string) (Entry, error) {
	var e Entry

	for kv := range strings.SplitSeq(s, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return e, fmt.Errorf("invalid prefix list entry %q: %q is not a key=value pair", s, kv)
		}

		switch k {
		case "action":
			e.Action = v
		case "prefix":
			e.Prefix = v
		case "seq", "ge", "le":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return e, fmt.Errorf("invalid prefix list entry %q: %s is not a number: %w", s, k, err)
			}

			switch k {
			case "seq":
				e.Seq = &n
			case "ge":
				e.GE = &n
			default:
				e.LE = &n
			}
		}
	}

	return e, nil
}
