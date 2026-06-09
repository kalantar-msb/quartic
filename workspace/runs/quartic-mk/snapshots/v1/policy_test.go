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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/llm-d/llm-d-router/pkg/epp/framework/interface/flowcontrol"
)

const epsilon = 1e-9

func TestFactory_NilConfig(t *testing.T) {
	t.Parallel()

	p, err := Factory("quartic-ceiling", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, p)

	policy, ok := p.(flowcontrol.UsageLimitPolicy)
	require.True(t, ok, "Factory output must implement UsageLimitPolicy")

	tn := policy.TypedName()
	assert.Equal(t, "quartic-ceiling", tn.Name)
	assert.NotEmpty(t, tn.Type)
}

func TestFactory_ComputeLimitDelegates(t *testing.T) {
	t.Parallel()

	p, err := Factory("quartic-ceiling", nil, nil)
	require.NoError(t, err)
	policy := p.(flowcontrol.UsageLimitPolicy)

	got := policy.ComputeLimit(context.Background(), 1.0, []int{100, -50})
	require.Len(t, got, 2)
	assert.InDelta(t, 1.0, got[0], epsilon)
	assert.InDelta(t, 0.0, got[1], epsilon)
}

func TestComputeQuarticCeilings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		saturation float64
		priorities []int
		want       []float64
	}{
		{
			name:       "empty priority domain returns empty slice",
			saturation: 0.5,
			priorities: nil,
			want:       []float64{},
		},
		{
			name:       "single band always uncapped regardless of saturation",
			saturation: 0.9,
			priorities: []int{100},
			want:       []float64{1.0},
		},
		{
			name:       "two bands at zero saturation: both uncapped",
			saturation: 0.0,
			priorities: []int{100, -50},
			want:       []float64{1.0, 1.0},
		},
		{
			name:       "two bands at half saturation: lowest gated by 0.5^4",
			saturation: 0.5,
			priorities: []int{100, -50},
			want:       []float64{1.0, 1.0 - 0.0625},
		},
		{
			name:       "two bands at full saturation: lowest fully gated",
			saturation: 1.0,
			priorities: []int{100, -50},
			want:       []float64{1.0, 0.0},
		},
		{
			name:       "four bands at full saturation: linear ramp to zero",
			saturation: 1.0,
			priorities: []int{300, 100, 0, -50},
			want:       []float64{1.0, 2.0 / 3.0, 1.0 / 3.0, 0.0},
		},
		{
			name:       "highest-priority band stays uncapped under any saturation",
			saturation: 0.8,
			priorities: []int{100, 0, -50},
			want:       []float64{1.0, 1.0 - 0.5*0.8*0.8*0.8*0.8, 1.0 - 0.8*0.8*0.8*0.8},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeQuarticCeilings(context.Background(), tc.saturation, tc.priorities)
			require.Len(t, got, len(tc.want))
			for i := range tc.want {
				assert.InDeltaf(t, tc.want[i], got[i], epsilon,
					"index %d: saturation=%v priorities=%v", i, tc.saturation, tc.priorities)
			}
		})
	}
}
