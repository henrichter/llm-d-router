/*
Copyright 2026 The Kubernetes Authors.

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

package concurrency

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNumericFields(t *testing.T) {
	load := &InFlightLoad{Tokens: 1000, Requests: 4}
	assert.Equal(t, map[string]float64{"tokens_inflight": 1000, "requests_inflight": 4}, load.NumericFields())

	uncached := &UncachedRequestTokens{Tokens: 250}
	assert.Equal(t, map[string]float64{"tokens_uncached": 250}, uncached.NumericFields())
}

func TestNumericFieldsOfNil(t *testing.T) {
	var load *InFlightLoad
	assert.Nil(t, load.NumericFields())

	var uncached *UncachedRequestTokens
	assert.Nil(t, uncached.NumericFields())
}
