package quarticceiling

import (
	"context"
	"encoding/json"

	"github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
	"github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/flowcontrol/usagelimits"
)

const PolicyType = "quartic-ceiling-policy"

func Factory(name string, _ json.RawMessage, _ plugin.Handle) (plugin.Plugin, error) {
	return usagelimits.NewPolicyFunc(name, computeQuarticCeilings), nil
}

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
