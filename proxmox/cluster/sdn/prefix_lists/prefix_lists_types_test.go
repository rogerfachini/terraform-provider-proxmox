/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package prefix_lists

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntryRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want Entry
		out  string
	}{
		{
			"key order as returned by PVE",
			"action=permit,prefix=10.0.0.0/16,le=32,seq=5",
			Entry{Action: "permit", Prefix: "10.0.0.0/16", LE: new(int64(32)), Seq: new(int64(5))},
			"action=permit,prefix=10.0.0.0/16,le=32,seq=5",
		},
		{
			"minimal entry",
			"prefix=2001:db8::/32,action=deny",
			Entry{Action: "deny", Prefix: "2001:db8::/32"},
			"action=deny,prefix=2001:db8::/32",
		},
		{
			"ge and le",
			"seq=10,action=permit,prefix=10.0.0.0/8,ge=24,le=28",
			Entry{Action: "permit", Prefix: "10.0.0.0/8", GE: new(int64(24)), LE: new(int64(28)), Seq: new(int64(10))},
			"action=permit,prefix=10.0.0.0/8,ge=24,le=28,seq=10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseEntry(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.out, got.String())
		})
	}
}

func TestParseEntryErrors(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"permit", "action=permit,prefix=10.0.0.0/8,le=x"} {
		_, err := ParseEntry(in)
		require.Error(t, err, in)
	}
}
