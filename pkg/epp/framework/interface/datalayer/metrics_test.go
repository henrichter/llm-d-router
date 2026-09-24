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

package datalayer

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestMetricsClone(t *testing.T) {
	m := &Metrics{
		ActiveModels:            map[string]int{"modelA": 1},
		WaitingModels:           map[string]int{"modelB": 2},
		MaxActiveModels:         5,
		RunningRequestsSize:     3,
		WaitingQueueSize:        7,
		KVCacheUsagePercent:     42.5,
		KvCacheMaxTokenCapacity: 2048,
		UpdateTime:              time.Now(),
	}

	clone := m.Clone()
	if diff := cmp.Diff(m, clone); diff != "" {
		t.Errorf("Unexpected output (-want +got): %v", diff)
	}

	// ensure maps are cloned and not sharing pointers
	clone.ActiveModels["modelA"] = 42
	assert.Equal(t, 1, m.ActiveModels["modelA"], "mutating clone should not affect original")
	clone.WaitingModels["modelB"] = 99
	assert.Equal(t, 2, m.WaitingModels["modelB"], "mutating clone should not affect original")
}

func TestMetricsCloneOfNil(t *testing.T) {
	var m *Metrics
	assert.Nil(t, m.Clone())
}

func TestMetricsToString(t *testing.T) {
	m := NewMetrics()
	assert.NotEmpty(t, m.String())

	var none *Metrics
	assert.Equal(t, "", none.String())
}

func TestMetricsNumericFields(t *testing.T) {
	m := &Metrics{
		RunningRequestsSize:     2,
		WaitingQueueSize:        3,
		KVCacheUsagePercent:     0.5,
		CacheBlockSize:          16,
		CacheNumBlocks:          100,
		KvCacheMaxTokenCapacity: 1600,
		MaxActiveModels:         4,
	}
	assert.Equal(t, map[string]float64{
		"requests_running":  2,
		"requests_waiting":  3,
		"cache_usage":       0.5,
		"cache_block_size":  16,
		"cache_max_blocks":  100,
		"cache_max_tokens":  1600,
		"models_max_active": 4,
	}, m.NumericFields())
}

func TestMetricsNumericFieldsOfNil(t *testing.T) {
	var m *Metrics
	assert.Nil(t, m.NumericFields())
}
