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

// Package minscore implements a scheduling picker that selects the endpoint(s) with the lowest
// score calculated during the scoring phase.
//
// For detailed behavioral intent and configuration, see the package README.
package minscore

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/log"

	logutil "github.com/llm-d/llm-d-router/pkg/common/observability/logging"
	fwkplugin "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
	fwksched "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/scheduling"
	"github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/scheduling/picker"
)

const (
	// MinScorePickerType is the registered name of the min score picker plugin.
	MinScorePickerType = "min-score-picker"
)

// compile-time type validation
var _ fwksched.Picker = &MinScorePicker{}

// MinScorePickerFactory defines the factory function for MinScorePicker.
func MinScorePickerFactory(name string, rawParameters *json.Decoder, _ fwkplugin.Handle) (fwkplugin.Plugin, error) {
	parameters := picker.PickerParameters{MaxNumOfEndpoints: picker.DefaultMaxNumOfEndpoints}
	if rawParameters != nil {
		if err := rawParameters.Decode(&parameters); err != nil {
			return nil, fmt.Errorf("failed to parse the parameters of the '%s' picker - %w", MinScorePickerType, err)
		}
	}

	return NewMinScorePicker(parameters.MaxNumOfEndpoints).WithName(name), nil
}

// NewMinScorePicker initializes a new MinScorePicker and returns its pointer.
func NewMinScorePicker(maxNumOfEndpoints int) *MinScorePicker {
	if maxNumOfEndpoints <= 0 {
		maxNumOfEndpoints = picker.DefaultMaxNumOfEndpoints // on invalid configuration value, fallback to default value
	}

	return &MinScorePicker{
		typedName:         fwkplugin.TypedName{Type: MinScorePickerType, Name: MinScorePickerType},
		maxNumOfEndpoints: maxNumOfEndpoints,
	}
}

// MinScorePicker picks endpoint(s) with the lowest score calculated during the scoring phase.
type MinScorePicker struct {
	typedName         fwkplugin.TypedName
	maxNumOfEndpoints int // maximum number of endpoints to pick
}

// WithName sets the picker's name
func (p *MinScorePicker) WithName(name string) *MinScorePicker {
	p.typedName.Name = name
	return p
}

// TypedName returns the type and name tuple of this plugin instance.
func (p *MinScorePicker) TypedName() fwkplugin.TypedName {
	return p.typedName
}

// Pick selects the endpoint(s) with the lowest score calculated during the scoring phase.
func (p *MinScorePicker) Pick(ctx context.Context, scoredEndpoints []*fwksched.ScoredEndpoint) *fwksched.ProfileRunResult {
	log.FromContext(ctx).V(logutil.DEBUG).Info("Selecting endpoints from candidates sorted by min score", "max-num-of-endpoints", p.maxNumOfEndpoints,
		"num-of-candidates", len(scoredEndpoints), "scored-endpoints", scoredEndpoints)

	// Shuffle in-place - needed for random tie break when scores are equal
	picker.ShuffleScoredEndpoints(scoredEndpoints)

	slices.SortStableFunc(scoredEndpoints, func(i, j *fwksched.ScoredEndpoint) int { // lowest score first
		if i.Score < j.Score {
			return -1
		}
		if i.Score > j.Score {
			return 1
		}
		return 0
	})

	// if we have enough endpoints to return keep only the "maxNumOfEndpoints" lowest scored endpoints
	if p.maxNumOfEndpoints < len(scoredEndpoints) {
		scoredEndpoints = scoredEndpoints[:p.maxNumOfEndpoints]
	}

	targetEndpoints := make([]fwksched.Endpoint, len(scoredEndpoints))
	for i, scoredEndpoint := range scoredEndpoints {
		targetEndpoints[i] = scoredEndpoint
	}

	return &fwksched.ProfileRunResult{TargetEndpoints: targetEndpoints}
}
