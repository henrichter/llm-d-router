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

package cel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8stypes "k8s.io/apimachinery/pkg/types"

	fwkdl "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/datalayer"
	fwksched "github.com/llm-d/llm-d-router/pkg/epp/framework/interface/scheduling"
	attrconcurrency "github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/datalayer/attribute/concurrency"
	attrmetrics "github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/datalayer/attribute/metrics"
	attrprefix "github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/datalayer/attribute/prefix"
)

// testProducer scopes the typed test attributes, in DataKey "Type/Producer" form.
const testProducer = "inflight-load-producer"

// newEndpoint builds an endpoint with typed producer state, a metrics
// snapshot, and bare custom scalars.
func newEndpoint(name string, uncached, inflightTokens, inflightRequests int64, running, waiting int,
	custom map[string]float64,
) fwksched.Endpoint {
	attrs := fwkdl.NewAttributes()
	attrs.Put(attrconcurrency.UncachedRequestTokensDataKey.WithNonEmptyProducerName(testProducer).String(),
		&attrconcurrency.UncachedRequestTokens{Tokens: uncached})
	attrs.Put(attrconcurrency.InFlightLoadDataKey.WithNonEmptyProducerName(testProducer).String(),
		&attrconcurrency.InFlightLoad{Tokens: inflightTokens, Requests: inflightRequests})
	for key, value := range custom {
		attrs.Put(key, attrmetrics.ScalarMetricValue(value))
	}
	metrics := &fwkdl.Metrics{RunningRequestsSize: running, WaitingQueueSize: waiting}
	return fwksched.NewEndpoint(&fwkdl.EndpointMetadata{ID: k8stypes.NamespacedName{Name: name}}, metrics, attrs)
}

func TestCombine(t *testing.T) {
	ep1 := newEndpoint("pod1", 100, 1000, 4, 2, 2, map[string]float64{"my_signal": 3})
	ep2 := newEndpoint("pod2", 50, 2000, 8, 5, 3, map[string]float64{"my_signal": 4})

	tests := []struct {
		name       string
		expression string
		results    []fwksched.ScorerResult
		endpoints  []fwksched.Endpoint
		want       map[fwksched.Endpoint]float64
	}{
		{
			name:       "scores only, multiplicative",
			expression: "scores['a'] * scores['b']",
			results: []fwksched.ScorerResult{
				{Name: "a", Scores: map[fwksched.Endpoint]float64{ep1: 0.5, ep2: 0.2}},
				{Name: "b", Scores: map[fwksched.Endpoint]float64{ep1: 0.4, ep2: 1.0}},
			},
			endpoints: []fwksched.Endpoint{ep1, ep2},
			want:      map[fwksched.Endpoint]float64{ep1: 0.2, ep2: 0.2},
		},
		{
			name:       "scores only, additive",
			expression: "scores['a'] + scores['b']",
			results: []fwksched.ScorerResult{
				{Name: "a", Scores: map[fwksched.Endpoint]float64{ep1: 0.5, ep2: 0.2}},
				{Name: "b", Scores: map[fwksched.Endpoint]float64{ep1: 0.4, ep2: 1.0}},
			},
			endpoints: []fwksched.Endpoint{ep1, ep2},
			want:      map[fwksched.Endpoint]float64{ep1: 0.9, ep2: 1.2},
		},
		{
			name:       "data only, uncached tokens times batch size plus one",
			expression: "data['inflight-load-producer']['tokens_uncached'] * (data['metrics']['requests_running'] + data['metrics']['requests_waiting'] + 1.0)",
			results:    nil,
			endpoints:  []fwksched.Endpoint{ep1, ep2},
			want:       map[fwksched.Endpoint]float64{ep1: 500, ep2: 450},
		},
		{
			name:       "inflight bucket signals",
			expression: "data['inflight-load-producer']['tokens_inflight'] + data['inflight-load-producer']['tokens_uncached']",
			results:    nil,
			endpoints:  []fwksched.Endpoint{ep1, ep2},
			want:       map[fwksched.Endpoint]float64{ep1: 1100, ep2: 2050},
		},
		{
			name:       "custom bucket signals",
			expression: "data['custom']['my_signal'] * 2.0",
			results:    nil,
			endpoints:  []fwksched.Endpoint{ep1, ep2},
			want:       map[fwksched.Endpoint]float64{ep1: 6, ep2: 8},
		},
		{
			name:       "mixed scores and data",
			expression: "scores['a'] * (data['metrics']['requests_running'] + data['metrics']['requests_waiting'] + 1.0)",
			results: []fwksched.ScorerResult{
				{Name: "a", Scores: map[fwksched.Endpoint]float64{ep1: 0.5, ep2: 0.25}},
			},
			endpoints: []fwksched.Endpoint{ep1, ep2},
			want:      map[fwksched.Endpoint]float64{ep1: 2.5, ep2: 2.25},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			combiner, err := NewCombiner("test", Parameters{Expression: test.expression})
			require.NoError(t, err)

			got := combiner.Combine(context.Background(), test.results, test.endpoints)

			require.Len(t, got, len(test.want))
			for endpoint, want := range test.want {
				assert.InDelta(t, want, got[endpoint], 1e-9, "endpoint %s", endpoint.GetMetadata().ID)
			}
		})
	}
}

// TestCombineBucketsFollowInstanceName verifies the data[] bucket tracks the
// attribute's producer instance: the same signal under a renamed producer is
// only reachable under the new bucket.
func TestCombineBucketsFollowInstanceName(t *testing.T) {
	attrs := fwkdl.NewAttributes()
	attrs.Put(attrconcurrency.UncachedRequestTokensDataKey.WithNonEmptyProducerName("short").String(),
		&attrconcurrency.UncachedRequestTokens{Tokens: 7})
	ep := fwksched.NewEndpoint(&fwkdl.EndpointMetadata{ID: k8stypes.NamespacedName{Name: "pod1"}},
		&fwkdl.Metrics{}, attrs)

	renamed, err := NewCombiner("test", Parameters{Expression: "data['short']['tokens_uncached'] * 2.0"})
	require.NoError(t, err)
	got := renamed.Combine(context.Background(), nil, []fwksched.Endpoint{ep})
	require.Len(t, got, 1)
	assert.InDelta(t, 14.0, got[ep], 1e-9)

	original, err := NewCombiner("test", Parameters{Expression: "data['inflight-load-producer']['tokens_uncached'] * 2.0"})
	require.NoError(t, err)
	assert.Empty(t, original.Combine(context.Background(), nil, []fwksched.Endpoint{ep}),
		"stale producer bucket must not resolve")
}

// TestCombinePrefixBuckets verifies prefix state resolves under its
// producer's bucket, keeping approx and precise comparable in one expression.
func TestCombinePrefixBuckets(t *testing.T) {
	attrs := fwkdl.NewAttributes()
	attrs.Put(attrprefix.PrefixCacheMatchInfoDataKey.WithNonEmptyProducerName("approx").String(),
		attrprefix.NewPrefixCacheMatchInfo(6, 10, 16))
	attrs.Put(attrprefix.PrefixCacheMatchInfoDataKey.WithNonEmptyProducerName("precise").String(),
		attrprefix.NewPrefixCacheMatchInfo(4, 10, 16))
	ep := fwksched.NewEndpoint(&fwkdl.EndpointMetadata{ID: k8stypes.NamespacedName{Name: "pod1"}},
		&fwkdl.Metrics{}, attrs)

	combiner, err := NewCombiner("test", Parameters{
		Expression: "data['approx']['tokens_cached'] - data['precise']['tokens_cached']",
	})
	require.NoError(t, err)
	got := combiner.Combine(context.Background(), nil, []fwksched.Endpoint{ep})
	require.Len(t, got, 1)
	assert.InDelta(t, 32.0, got[ep], 1e-9)
}

// TestCombineExcludesEndpointOnEvalError verifies that an endpoint whose
// expression references a data[] key it does not have is excluded, while a
// well-formed endpoint is scored.
func TestCombineExcludesEndpointOnEvalError(t *testing.T) {
	withKey := newEndpoint("pod1", 0, 0, 0, 3, 1, nil)
	withoutMetrics := fwksched.NewEndpoint(&fwkdl.EndpointMetadata{ID: k8stypes.NamespacedName{Name: "pod2"}},
		nil, fwkdl.NewAttributes())

	combiner, err := NewCombiner("test", Parameters{Expression: "data['metrics']['requests_running'] * 2.0"})
	require.NoError(t, err)

	got := combiner.Combine(context.Background(), nil, []fwksched.Endpoint{withKey, withoutMetrics})

	require.Len(t, got, 1)
	assert.InDelta(t, 6.0, got[withKey], 1e-9)
	_, ok := got[withoutMetrics]
	assert.False(t, ok, "endpoint missing the referenced data key must be excluded")
}

func TestNewCombinerErrors(t *testing.T) {
	tests := []struct {
		name       string
		expression string
	}{
		{name: "empty expression", expression: ""},
		{name: "compile error", expression: "scores['a'] * ("},
		{name: "unknown variable", expression: "bogus['a']"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCombiner("test", Parameters{Expression: test.expression})
			assert.Error(t, err)
		})
	}
}
