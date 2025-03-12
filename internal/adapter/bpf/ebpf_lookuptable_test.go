/*
 * Copyright 2025 Alexandre Mahdhaoui
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package bpfadapter_test

import (
	"fmt"
	"testing"

	bpfadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/bpf"
	"github.com/google/uuid"
)

func TestImprovedFibLookupTable(t *testing.T) {
	var (
		availableBackends []*bpfadapter.Backend

		n uint64           // number of available backends
		m bpfadapter.Prime // size of the
	)

	setup := func(t *testing.T) {
		t.Helper()

		availableBackends = make([]*bpfadapter.Backend, n)
		for i := range n {
			availableBackends[i] = bpfadapter.NewBackend(
				uuid.New(),
				bpfadapter.BackendSpec{},
				bpfadapter.BackendStatus{},
				false,
			)
		}
	}

	for _, prime := range []bpfadapter.Prime{
		bpfadapter.Prime307,
		bpfadapter.Prime4071,
		bpfadapter.Prime65497,
	} {
		t.Run(fmt.Sprintf("prime=%d", prime), func(t *testing.T) {
			m = prime

			for _, tc := range []struct {
				nBefore uint64
				nAfter  uint64
			}{
				{nBefore: 3, nAfter: 2},
				{nBefore: 3, nAfter: 5},
				{nBefore: 29, nAfter: 28},
				{nBefore: 29, nAfter: 31},
				{nBefore: 299, nAfter: 298},
				{nBefore: 299, nAfter: 301},
			} {
				t.Run(
					fmt.Sprintf("nBefore=%d,nAfter=%d", tc.nBefore, tc.nAfter),
					func(t *testing.T) {
						if tc.nBefore > tc.nAfter {
							n = tc.nBefore
						} else {
							n = tc.nAfter
						}

						setup(t)

						backends := availableBackends[0:tc.nBefore]
						_, before := bpfadapter.ImprovedFibLookupTable(backends, m)

						backends = availableBackends[0:tc.nAfter]
						_, after := bpfadapter.ImprovedFibLookupTable(backends, m)

						// Calculate percentage of equal entries.
						idem := 0
						for i := range m {
							if before[i] == after[i] {
								idem++
							}
						}

						t.Logf("%02f%%", float64(idem)/float64(m))
					},
				)
			}
		})
	}
}
