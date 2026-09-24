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

package scheduling

import (
	"context"

	fwkplugin "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
	fwksched "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/scheduling"
)

// weightedSumCombinerType is the plugin type name of the default combiner.
const weightedSumCombinerType = "weighted-sum-combiner"

// defaultWeightedSumCombiner is the combiner a profile uses when none is
// configured. It reproduces the scheduler's historical scoring: each scorer's
// value is clamped to [0,1] and accumulated with its weight.
var defaultWeightedSumCombiner fwksched.Combiner = &weightedSumCombiner{
	typedName: fwkplugin.TypedName{Type: weightedSumCombinerType, Name: weightedSumCombinerType},
}

// weightedSumCombiner sums each scorer's clamped score times its weight.
type weightedSumCombiner struct {
	typedName fwkplugin.TypedName
}

func (c *weightedSumCombiner) TypedName() fwkplugin.TypedName {
	return c.typedName
}

func (c *weightedSumCombiner) Combine(_ context.Context, results []fwksched.ScorerResult,
	endpoints []fwksched.Endpoint,
) map[fwksched.Endpoint]float64 {
	combined := make(map[fwksched.Endpoint]float64, len(endpoints))
	for _, endpoint := range endpoints {
		combined[endpoint] = 0
	}
	for _, result := range results {
		for endpoint, score := range result.Scores {
			combined[endpoint] += enforceScoreRange(score) * result.Weight
		}
	}
	return combined
}
