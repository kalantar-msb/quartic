package quarticceiling

import (
	"context"
	"encoding/json"

	"github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
	"github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/flowcontrol/usagelimits"
)

const ControlPolicyType = "constant-ceiling-control"

func ControlFactory(name string, _ json.RawMessage, _ plugin.Handle) (plugin.Plugin, error) {
	return usagelimits.NewPolicyFunc(name, computeConstantCeilings), nil
}

func computeConstantCeilings(_ context.Context, _ float64, priorities []int) []float64 {
	ceilings := make([]float64, len(priorities))
	for i := range ceilings {
		ceilings[i] = 1.0
	}
	return ceilings
}
