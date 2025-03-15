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

type (
	LookupTableFunc func(availableBackends []*bpfadapter.Backend, lookupTableLength bpfadapter.Prime) (keys []uint64, values []string)
	Scenario        struct {
		nBefore uint64
		nAfter  uint64
	}
)

// go test -v -cpu=`nproc` -bench=. --benchmem --benchtime=1s -count=40 --timeout=180m ./internal/adapter/bpf/ebpf_lookuptable_test.go  | tee .ignore.benchmark
func BenchmarkLookupTable(b *testing.B) {
	var (
		beCount uint64           // number of available backends
		lupLen  bpfadapter.Prime // size of the

		availableBackends []*bpfadapter.Backend
	)

	setup := func(b *testing.B) {
		b.Helper()

		availableBackends = make([]*bpfadapter.Backend, beCount)
		for i := range beCount {
			availableBackends[i] = bpfadapter.NewBackend(
				uuid.New(),
				bpfadapter.BackendSpec{},
				bpfadapter.BackendStatus{},
				false,
			)
		}
	}

	funcs := map[string]LookupTableFunc{
		"NaiveSimple":  bpfadapter.SimpleLookupTable,
		"NaiveFib":     bpfadapter.NaiveFibLookupTable,
		"RobustFib":    bpfadapter.RobustFibLookupTable,
		"RobustSimple": bpfadapter.RobustSimpleLookupTable,
	}

	primes := []bpfadapter.Prime{
		bpfadapter.Prime(23),
		bpfadapter.Prime(47),
		bpfadapter.Prime307,
		bpfadapter.Prime4071,
		bpfadapter.Prime65497,
	}

	scenarios := []Scenario{
		// Lost one replica.
		{nBefore: 3, nAfter: 2},
		// Scaled up: add 2 new replicas.
		{nBefore: 3, nAfter: 5},

		// Lost one replica.
		{nBefore: 7, nAfter: 6},
		// Scaled up: add 2 new replicas.
		{nBefore: 7, nAfter: 9},

		// Lost 3 replicas
		{nBefore: 29, nAfter: 26},
		// Scaled up: add 6 new replicas
		{nBefore: 29, nAfter: 35},

		// Lost 5 replicas
		{nBefore: 255, nAfter: 250},
		// Scaled up: add 10 new replicas
		{nBefore: 255, nAfter: 299},
	}

	for fName, f := range funcs {
		name := fmt.Sprintf("func=%s", fName)

		for _, p := range primes {
			name := fmt.Sprintf("%s/prime=%d", name, p)

			for _, sc := range scenarios {
				if uint64(p) < sc.nBefore || uint64(p) < sc.nAfter {
					// skip benchmarking if the number of backend is bigger than the lookup table length.
					continue
				}

				name := fmt.Sprintf("%s/nBefore=%d/nAfter=%d", name, sc.nBefore, sc.nAfter)
				b.Run(name, func(b *testing.B) {
					lupLen = p
					beCount = sc.nBefore
					if sc.nBefore < sc.nAfter {
						beCount = sc.nAfter
					}

					b.ResetTimer()
					for b.Loop() {
						setup(b)

						backends := availableBackends[0:sc.nBefore]
						_, before := f(backends, lupLen)

						backends = availableBackends[0:sc.nAfter]
						_, after := f(backends, lupLen)

						// Calculate rate of unchanged entries.
						idem := 0
						for i := range lupLen {
							if before[i] == after[i] {
								idem++
							}
						}

						unchangedEntries := float64(idem) / float64(lupLen)
						b.ReportMetric(100*unchangedEntries, "%unchangedEntries/op")
					}
				})
			}
		}
	}
}
