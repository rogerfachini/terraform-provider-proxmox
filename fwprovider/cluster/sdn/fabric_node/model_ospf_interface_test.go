/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package fabric_node

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSPFInterfacePropertyString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want ospfInterfaceModel
		out  string
	}{
		{
			"name only", "name=nic0.53",
			ospfInterfaceModel{Name: types.StringValue("nic0.53"), IP: types.StringNull(), NetworkType: types.StringNull()},
			"name=nic0.53",
		},
		{
			"all options, any key order", "network_type=point-to-point,ip=10.0.100.1/31,name=nic0",
			ospfInterfaceModel{
				Name: types.StringValue("nic0"), IP: types.StringValue("10.0.100.1/31"), NetworkType: types.StringValue("point-to-point"),
			},
			"name=nic0,ip=10.0.100.1/31,network_type=point-to-point",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseOSPFInterface(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.out, got.toAPI())
		})
	}

	for _, in := range []string{"nic0", "ip=10.0.0.1/31"} {
		_, err := parseOSPFInterface(in)
		require.Error(t, err, in)
	}
}

// The same API state can be held by interface_names or by interfaces; the one the configuration uses must survive.
func TestOSPFModelShapeLike(t *testing.T) {
	t.Parallel()

	read := func(raw ...string) *ospfModel {
		var diags diag.Diagnostics

		m := &ospfModel{}
		m.InterfaceNames, _ = m.toInterfaceNamesFromInterfaces(raw)
		m.Interfaces = ospfInterfacesFromAPI(raw, &diags)
		require.False(t, diags.HasError())

		return m
	}

	t.Run("prior used interface_names", func(t *testing.T) {
		t.Parallel()

		prior := read("name=nic0")
		prior.Interfaces = types.SetNull(prior.Interfaces.ElementType(t.Context()))

		m := read("name=nic0", "name=nic1")
		m.shapeLike(prior)

		assert.True(t, m.Interfaces.IsNull())
		assert.Len(t, m.InterfaceNames.Elements(), 2)
	})

	t.Run("prior used interfaces", func(t *testing.T) {
		t.Parallel()

		prior := read("name=nic0")
		prior.InterfaceNames = types.SetNull(types.StringType)

		m := read("name=nic0,network_type=point-to-point")
		m.shapeLike(prior)

		assert.True(t, m.InterfaceNames.IsNull())
		assert.Len(t, m.Interfaces.Elements(), 1)
	})

	t.Run("import picks interface_names when no interface has options", func(t *testing.T) {
		t.Parallel()

		m := read("name=nic0", "name=nic1")
		m.shapeLike(nil)

		assert.True(t, m.Interfaces.IsNull())
		assert.False(t, m.InterfaceNames.IsNull())
	})

	t.Run("import picks interfaces when an interface has options", func(t *testing.T) {
		t.Parallel()

		m := read("name=nic0", "name=nic1,ip=10.0.100.3/31")
		m.shapeLike(nil)

		assert.True(t, m.InterfaceNames.IsNull())
		assert.Len(t, m.Interfaces.Elements(), 2)
	})
}
