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
	"fmt"
	"maps"
	"time"
)

// Metrics holds the latest metrics snapshot scraped from a pod.
type Metrics struct {
	// ActiveModels is a set of models(including LoRA adapters) that are currently cached to GPU.
	ActiveModels  map[string]int
	WaitingModels map[string]int
	// MaxActiveModels is the maximum number of models that can be loaded to GPU.
	MaxActiveModels         int
	RunningRequestsSize     int
	WaitingQueueSize        int
	KVCacheUsagePercent     float64
	KvCacheMaxTokenCapacity int
	CacheBlockSize          int
	// Number of GPU blocks in the model server for KV Cache.
	CacheNumBlocks int

	// UpdateTime records the last time when the metrics were updated.
	UpdateTime time.Time
}

// NewMetrics initializes a new empty Metrics object.
func NewMetrics() *Metrics {
	return &Metrics{
		ActiveModels:  make(map[string]int),
		WaitingModels: make(map[string]int),
	}
}

// String returns a string with all Metric information
func (m *Metrics) String() string {
	if m == nil {
		return ""
	}
	return fmt.Sprintf("%+v", *m)
}

// Clone creates a copy of Metrics and returns its pointer.
// Clone returns nil if the object being cloned is nil.
func (m *Metrics) Clone() *Metrics {
	if m == nil {
		return nil
	}
	activeModels := make(map[string]int, len(m.ActiveModels))
	maps.Copy(activeModels, m.ActiveModels)
	waitingModels := make(map[string]int, len(m.WaitingModels))
	maps.Copy(waitingModels, m.WaitingModels)
	return &Metrics{
		ActiveModels:            activeModels,
		WaitingModels:           waitingModels,
		MaxActiveModels:         m.MaxActiveModels,
		RunningRequestsSize:     m.RunningRequestsSize,
		WaitingQueueSize:        m.WaitingQueueSize,
		KVCacheUsagePercent:     m.KVCacheUsagePercent,
		KvCacheMaxTokenCapacity: m.KvCacheMaxTokenCapacity,
		CacheBlockSize:          m.CacheBlockSize,
		CacheNumBlocks:          m.CacheNumBlocks,
		UpdateTime:              m.UpdateTime,
	}
}

// NumericFields reports raw magnitudes; derived combinations (e.g. total
// batch size) belong in the expression, not here.
func (m *Metrics) NumericFields() map[string]float64 {
	if m == nil {
		return nil
	}
	return map[string]float64{
		"requests_running":  float64(m.RunningRequestsSize),
		"requests_waiting":  float64(m.WaitingQueueSize),
		"cache_usage":       m.KVCacheUsagePercent,
		"cache_block_size":  float64(m.CacheBlockSize),
		"cache_max_blocks":  float64(m.CacheNumBlocks),
		"cache_max_tokens":  float64(m.KvCacheMaxTokenCapacity),
		"models_max_active": float64(m.MaxActiveModels),
	}
}

var _ NumericView = (*Metrics)(nil)
