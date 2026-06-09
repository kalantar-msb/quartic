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

// Package quarticceiling implements a parameter-free UsageLimitPolicy that
// computes per-band dispatch ceilings as ceiling[i] = 1.0 - i/(N-1) * sat^4,
// where i is the band position (0 = highest priority), N is the number of
// active priority bands, and sat is the pool-wide saturation in [0.0, 1.0].
package quarticceiling

import (
	"context"
	"encoding/json"

	"github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
	"github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/flowcontrol/usagelimits"
)

const PolicyType = "quartic-ceiling-policy"

// Factory constructs a quartic-ceiling-policy plugin instance. The policy is
// parameter-free; rawConfig is ignored.
func Factory(name string, _ *json.Decoder, _ plugin.Handle) (plugin.Plugin, error) {
	return usagelimits.NewPolicyFunc(name, computeQuarticCeilings), nil
}

// computeQuarticCeilings returns ceilings[i] = 1.0 - i/(N-1) * sat^4 for each
// active priority band. The highest-priority band (i=0) is always uncapped
// (ceiling 1.0); the lowest-priority band (i=N-1) is gated by sat^4.
func computeQuarticCeilings(_ context.Context, saturation float64, priorities []int) []float64 {
	n := len(priorities)
	ceilings := make([]float64, n)
	if n <= 1 {
		if n == 1 {
			ceilings[0] = 1.0
		}
		return ceilings
	}
	sat4 := saturation * saturation * saturation * saturation
	for i := range priorities {
		ceilings[i] = 1.0 - float64(i)/float64(n-1)*sat4
	}
	return ceilings
}
