/*
Copyright 2025 The Kubernetes Authors.

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

package quarticceiling

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fwkplugin "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
)

func TestPolicyType(t *testing.T) {
	assert.Equal(t, "quartic-ceiling-policy", PolicyType)
}

func TestFactory(t *testing.T) {
	p, err := Factory("quartic-ceiling", fwkplugin.StrictDecoder(nil), nil)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "quartic-ceiling", p.TypedName().Name)
}

func TestFactory_NilDecoder(t *testing.T) {
	// The plugin is parameter-free; a nil decoder must not error.
	p, err := Factory("quartic-ceiling", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestComputeQuarticCeilings_NoBands(t *testing.T) {
	got := computeQuarticCeilings(context.Background(), 0.5, nil)
	assert.Empty(t, got)
}

func TestComputeQuarticCeilings_SingleBand(t *testing.T) {
	// With N=1 the i/(N-1) term is undefined; the policy returns 1.0
	// (no gating possible without a relative comparison).
	got := computeQuarticCeilings(context.Background(), 0.9, []int{100})
	require.Len(t, got, 1)
	assert.Equal(t, 1.0, got[0])
}

func TestComputeQuarticCeilings_TwoBands_NoSaturation(t *testing.T) {
	got := computeQuarticCeilings(context.Background(), 0.0, []int{100, -50})
	require.Len(t, got, 2)
	assert.Equal(t, 1.0, got[0])
	assert.Equal(t, 1.0, got[1])
}

func TestComputeQuarticCeilings_TwoBands_FullSaturation(t *testing.T) {
	got := computeQuarticCeilings(context.Background(), 1.0, []int{100, -50})
	require.Len(t, got, 2)
	// Critical band always protected.
	assert.Equal(t, 1.0, got[0])
	// Sheddable: 1 - 1/(2-1) * 1^4 = 0.0
	assert.InDelta(t, 0.0, got[1], 1e-12)
}

func TestComputeQuarticCeilings_TwoBands_KnownPoints(t *testing.T) {
	// Reference points from the algorithm card:
	//   sat=0.5 -> sheddable 0.9375
	//   sat=0.8 -> sheddable 0.5904
	//   sat=0.9 -> sheddable 0.3439
	cases := []struct{ sat, want float64 }{
		{0.5, 1.0 - math.Pow(0.5, 4)},
		{0.8, 1.0 - math.Pow(0.8, 4)},
		{0.9, 1.0 - math.Pow(0.9, 4)},
	}
	for _, tc := range cases {
		got := computeQuarticCeilings(context.Background(), tc.sat, []int{100, -50})
		require.Len(t, got, 2)
		assert.Equal(t, 1.0, got[0])
		assert.InDelta(t, tc.want, got[1], 1e-12)
	}
}

func TestComputeQuarticCeilings_MultiBand_Monotonic(t *testing.T) {
	// 4 bands at mid-saturation: ceilings strictly decrease, top stays at 1.0,
	// bottom equals 1 - sat^4 (i/(N-1) = 1).
	priorities := []int{100, 50, 0, -50}
	const sat = 0.7
	got := computeQuarticCeilings(context.Background(), sat, priorities)
	require.Len(t, got, 4)

	assert.Equal(t, 1.0, got[0])
	for i := 1; i < len(got); i++ {
		assert.Less(t, got[i], got[i-1],
			"ceilings must be strictly decreasing under positive saturation")
	}

	sat4 := math.Pow(sat, 4)
	assert.InDelta(t, 1.0-(1.0/3.0)*sat4, got[1], 1e-12)
	assert.InDelta(t, 1.0-(2.0/3.0)*sat4, got[2], 1e-12)
	assert.InDelta(t, 1.0-sat4, got[3], 1e-12)
}

func TestComputeQuarticCeilings_FactoryRoundTrip(t *testing.T) {
	// End-to-end: build via Factory and exercise ComputeLimit through the
	// flowcontrol.UsageLimitPolicy interface returned by NewPolicyFunc.
	p, err := Factory("quartic-ceiling", nil, nil)
	require.NoError(t, err)

	type computer interface {
		ComputeLimit(ctx context.Context, saturation float64, priorities []int) []float64
	}
	c, ok := p.(computer)
	require.True(t, ok, "plugin must implement ComputeLimit")

	got := c.ComputeLimit(context.Background(), 0.8, []int{100, -50})
	require.Len(t, got, 2)
	assert.Equal(t, 1.0, got[0])
	assert.InDelta(t, 1.0-math.Pow(0.8, 4), got[1], 1e-12)
}
