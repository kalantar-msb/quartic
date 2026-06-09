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

// Package quarticceiling implements a parameter-free per-band dispatch
// ceiling UsageLimitPolicy:
//
//	ceiling[i] = 1.0 - i/(N-1) * sat^4
//
// The highest-priority band (i=0) is always protected at ceiling 1.0;
// lower-priority bands are progressively gated as saturation rises.
// The fourth-power saturation term keeps gating dormant in steady state
// and engages aggressively only as the pool nears overload.
package quarticceiling

import (
	"context"
	"encoding/json"

	"github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
	"github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/flowcontrol/usagelimits"
)

const PolicyType = "quartic-ceiling-policy"

// Factory creates a quartic-ceiling UsageLimitPolicy. The plugin takes no
// configuration; the decoder argument is ignored.
func Factory(name string, _ *json.Decoder, _ plugin.Handle) (plugin.Plugin, error) {
	return usagelimits.NewPolicyFunc(name, computeQuarticCeilings), nil
}

// computeQuarticCeilings returns ceiling[i] = 1.0 - i/(N-1) * sat^4 for
// each active priority band, with N = len(priorities). With 0 or 1 bands
// no relative ceiling is defined, so all returned ceilings are 1.0.
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
