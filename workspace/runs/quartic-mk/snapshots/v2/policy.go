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

// Package constantcontrol implements a parameter-free UsageLimitPolicy that
// returns ceiling[i] = 1.0 for every active priority band, regardless of
// pool-wide saturation. It is the control arm paired with quarticceiling for
// isolating framework overhead from algorithm effect.
package constantcontrol

import (
	"context"
	"encoding/json"

	"github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
	"github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/flowcontrol/usagelimits"
)

const PolicyType = "constant-ceiling-control"

// Factory constructs a constant-ceiling-control plugin instance. The policy is
// parameter-free; rawConfig is ignored.
func Factory(name string, _ *json.Decoder, _ plugin.Handle) (plugin.Plugin, error) {
	return usagelimits.NewPolicyFunc(name, computeConstantCeilings), nil
}

// computeConstantCeilings returns 1.0 for every priority band, independent of
// saturation. The result slice has the same length as priorities.
func computeConstantCeilings(_ context.Context, _ float64, priorities []int) []float64 {
	ceilings := make([]float64, len(priorities))
	for i := range ceilings {
		ceilings[i] = 1.0
	}
	return ceilings
}
