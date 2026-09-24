/*
Copyright 2026 The llm-d Authors.

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

// Package cel implements a scheduling combiner that folds per-scorer results
// and raw endpoint state into one score per endpoint using a CEL expression.
//
// The expression is evaluated once per endpoint against two map variables:
//
//	scores['scorer-name'] // that endpoint's output from the named scorer, [0,1]
//	data['producer']['signal'] // raw magnitudes grouped by producer instance
//
// Bucket catalog and configuration live in the package README.
package cel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	"sigs.k8s.io/controller-runtime/pkg/log"

	logutil "github.com/llm-d/llm-d-router/pkg/common/observability/logging"
	fwkdl "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/datalayer"
	fwkplugin "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
	fwksched "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/scheduling"
	attrmetrics "github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/datalayer/attribute/metrics"
)

const (
	// CombinerType is the registered name of the CEL combiner plugin.
	CombinerType = "cel-combiner"

	scoresVar = "scores"
	dataVar   = "data"

	// metricsBucket holds the endpoint metrics snapshot signals.
	metricsBucket = "metrics"
	// customBucket collects bare keys without a producer component.
	customBucket = "custom"
)

// compile-time type assertion
var _ fwksched.Combiner = &Combiner{}

// Parameters configures the CEL combiner.
type Parameters struct {
	// Expression is evaluated per endpoint against scores[] and the nested
	// data['producer']['signal'] maps and must evaluate to a number.
	Expression string `json:"expression"`
}

// CombinerFactory defines the factory function for the CEL combiner.
func CombinerFactory(name string, rawParameters *json.Decoder, _ fwkplugin.Handle) (fwkplugin.Plugin, error) {
	var params Parameters
	if rawParameters != nil {
		if err := rawParameters.Decode(&params); err != nil {
			return nil, fmt.Errorf("failed to parse the parameters of the '%s' combiner - %w", CombinerType, err)
		}
	}

	return NewCombiner(name, params)
}

// NewCombiner compiles the expression and returns a new CEL combiner. The
// expression is compiled once here; each Combine call reuses the program.
func NewCombiner(name string, params Parameters) (*Combiner, error) {
	if name == "" {
		name = CombinerType
	}
	if params.Expression == "" {
		return nil, errors.New("cel combiner requires a non-empty expression")
	}

	env, err := cel.NewEnv(
		cel.Variable(scoresVar, cel.MapType(cel.StringType, cel.DoubleType)),
		cel.Variable(dataVar, cel.MapType(cel.StringType, cel.MapType(cel.StringType, cel.DoubleType))),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create cel environment: %w", err)
	}

	ast, issues := env.Compile(params.Expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("failed to compile expression %q: %w", params.Expression, issues.Err())
	}
	prog, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("failed to create program for expression %q: %w", params.Expression, err)
	}

	return &Combiner{
		typedName:  fwkplugin.TypedName{Type: CombinerType, Name: name},
		expression: params.Expression,
		prog:       prog,
	}, nil
}

// Combiner folds per-scorer results and raw endpoint attributes into one score
// per endpoint by evaluating a CEL expression.
type Combiner struct {
	typedName  fwkplugin.TypedName
	expression string
	prog       cel.Program
}

// WithName sets the combiner's name.
func (c *Combiner) WithName(name string) *Combiner {
	c.typedName.Name = name
	return c
}

// TypedName returns the type and name tuple of this plugin instance.
func (c *Combiner) TypedName() fwkplugin.TypedName {
	return c.typedName
}

// Combine evaluates the configured expression for each endpoint. An endpoint
// whose evaluation errors is excluded from the result so it can never win the
// pick.
func (c *Combiner) Combine(ctx context.Context, results []fwksched.ScorerResult,
	endpoints []fwksched.Endpoint,
) map[fwksched.Endpoint]float64 {
	logger := log.FromContext(ctx)

	combined := make(map[fwksched.Endpoint]float64, len(endpoints))
	for _, endpoint := range endpoints {
		scores := make(map[string]float64, len(results))
		for _, result := range results {
			scores[result.Name] = result.Scores[endpoint]
		}

		value, err := c.eval(scores, dataMap(endpoint))
		if err != nil {
			logger.V(logutil.DEBUG).Info("excluding endpoint: cel evaluation failed",
				"expression", c.expression, "endpoint", endpoint.GetMetadata().ID, "error", err)
			continue
		}
		combined[endpoint] = value
	}

	return combined
}

// eval runs the compiled program against one endpoint's score and data maps and
// converts the result to float64.
func (c *Combiner) eval(scores map[string]float64, data map[string]map[string]float64) (float64, error) {
	out, _, err := c.prog.Eval(map[string]any{scoresVar: scores, dataVar: data})
	if err != nil {
		return 0, err
	}

	switch v := out.Value().(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("expression result %v is not numeric", out.Value())
	}
}

// dataMap builds the nested data[] variable. Views resolve against live
// values at Combine time; nothing is stored. Attribute keys in DataKey
// "Type/Producer" form bucket by the part after the last "/".
func dataMap(endpoint fwksched.Endpoint) map[string]map[string]float64 {
	data := map[string]map[string]float64{}
	put := func(bucket, signal string, value float64) {
		bucketMap, ok := data[bucket]
		if !ok {
			bucketMap = map[string]float64{}
			data[bucket] = bucketMap
		}
		bucketMap[signal] = value
	}
	for _, key := range endpoint.Keys() {
		bucket := customBucket
		signal := key
		// DataKey.String serializes to "DataType/ProducerName"; the producer
		// component scopes the bucket to the plugin instance.
		if i := strings.LastIndex(key, "/"); i >= 0 {
			bucket = key[i+1:]
			signal = key[:i]
		}
		raw, ok := endpoint.Get(key)
		if !ok {
			continue
		}
		if view, ok := raw.(fwkdl.NumericView); ok {
			for name, value := range view.NumericFields() {
				put(bucket, name, value)
			}
			continue
		}
		if value, ok := raw.(attrmetrics.ScalarMetricValue); ok {
			put(bucket, signal, float64(value))
		}
	}
	if metrics := endpoint.GetMetrics(); metrics != nil {
		for name, value := range metrics.NumericFields() {
			put(metricsBucket, name, value)
		}
	}
	return data
}
