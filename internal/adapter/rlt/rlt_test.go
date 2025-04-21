//go:build benchmark

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
package rltadapter_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	rltadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/rlt"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/google/uuid"
)

type (
	LookupTableFunc func(
		availableBackends []*types.Backend,
		lookupTableLength uint32,
	) []uint32

	Scenario struct {
		nBefore uint32
		nAfter  uint32
	}
)

// go test -v -cpu=`nproc` -bench=. --benchmem --benchtime=1s -count=40 --timeout=180m ./internal/adapter/bpf/ebpf_lookuptable_test.go  | tee .ignore.benchmark
func BenchmarkLookupTable(b *testing.B) {
	var (
		beCount           uint32 // number of available backends
		availableBackends []*types.Backend
	)

	setup := func(b *testing.B) {
		b.Helper()

		availableBackends = make([]*types.Backend, beCount)
		for i := range beCount {
			availableBackends[i] = types.NewBackend(
				uuid.New(),
				types.BackendSpec{},
				types.BackendStatus{},
			)
		}
	}

	funcs := map[string]LookupTableFunc{
		"NaiveSimple":  rltadapter.SimpleLookupTable,
		"NaiveFib":     rltadapter.NaiveFibLookupTable,
		"RobustFib":    rltadapter.RobustFibLookupTable,
		"RobustSimple": rltadapter.RobustSimpleLookupTable,
		"RevCoord":     rltadapter.ReverseCoordinatesLookupTable,
	}

	primes := []uint32{13, 23, 47, 307}

	scenarios := []Scenario{
		{nBefore: 3, nAfter: 2},
		{nBefore: 3, nAfter: 5},

		{nBefore: 7, nAfter: 6},
		{nBefore: 7, nAfter: 9},

		{nBefore: 27, nAfter: 25},
		{nBefore: 27, nAfter: 30},
	}

	for fName, f := range funcs {
		name := fmt.Sprintf("func=%s", fName)

		for _, p := range primes {
			name := fmt.Sprintf("%s/prime=%d", name, p)

			for _, sc := range scenarios {
				if p < sc.nBefore || p < sc.nAfter {
					// skip benchmarking if the number of backend is bigger than the lookup table length.
					continue
				}

				name := fmt.Sprintf("%s/nBefore=%d/nAfter=%d", name, sc.nBefore, sc.nAfter)
				b.Run(name, func(b *testing.B) {
					beCount = max(sc.nBefore, sc.nAfter)

					b.ResetTimer()
					for b.Loop() {
						setup(b)

						before := f(nChooseK(b, sc.nBefore, availableBackends), p)
						after := f(nChooseK(b, sc.nAfter, availableBackends), p)

						// Calculate rate of unchanged entries.
						idem := 0
						for i := range p {
							if before[i] == after[i] {
								idem++
							}
						}

						unchangedEntries := float64(idem) / float64(p)
						b.ReportMetric(100*unchangedEntries, "%unchangedEntries/op")
					}
				})
			}
		}
	}
}

func nChooseK(b *testing.B, k uint32, sl []*types.Backend) []*types.Backend {
	b.Helper()

	out := make([]*types.Backend, k)
	perm := rand.Perm(len(sl))
	for i := range k {
		out[i] = sl[perm[i]]
	}

	slices.SortFunc(out, func(a, b *types.Backend) int {
		astr, bstr := a.Id.String(), b.Id.String()
		switch {
		case astr > bstr:
			return 1
		case astr < bstr:
			return -1
		default:
			return 0
		}
	})

	return out
}
