//go:build unit

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
package rltadapter

import (
	"testing"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeBackends creates n backends with deterministic UUIDs for reproducibility.
func makeBackends(n int) []*types.Backend {
	out := make([]*types.Backend, n)
	for i := range n {
		// Deterministic UUID: first 4 bytes encode the index, rest is zero-padded.
		var raw [16]byte
		raw[0] = byte(i >> 24)
		raw[1] = byte(i >> 16)
		raw[2] = byte(i >> 8)
		raw[3] = byte(i)
		// Set version and variant bits so it is a valid UUID v4.
		raw[6] = (raw[6] & 0x0f) | 0x40
		raw[8] = (raw[8] & 0x3f) | 0x80
		id, _ := uuid.FromBytes(raw[:])
		out[i] = types.NewBackend(id, types.BackendSpec{}, types.BackendStatus{})
	}
	return out
}

// makeBackendsNonPointer creates n backends as a value slice (for ShardedLookupTable).
func makeBackendsNonPointer(n int) []types.Backend {
	ptrs := makeBackends(n)
	out := make([]types.Backend, n)
	for i, p := range ptrs {
		out[i] = *p
	}
	return out
}

// assertValidTable checks generic properties that hold for every lookup table:
//   - length equals m
//   - every entry is in [0, nBackend)
func assertValidTable(t *testing.T, table []uint32, m uint32, nBackend uint32) {
	t.Helper()
	require.Len(t, table, int(m), "table length must equal m")
	for i, v := range table {
		assert.Less(t, v, nBackend, "table[%d]=%d must be < nBackend=%d", i, v, nBackend)
	}
}

// backendIndicesInTable returns the set of distinct backend indices in the table.
func backendIndicesInTable(table []uint32) map[uint32]struct{} {
	s := make(map[uint32]struct{})
	for _, v := range table {
		s[v] = struct{}{}
	}
	return s
}

// ---------------------------------------------------------------------------
// SimpleLookupTable
// ---------------------------------------------------------------------------

func TestSimpleLookupTable(t *testing.T) {
	t.Run("basic correctness with 3 backends and prime 307", func(t *testing.T) {
		backends := makeBackends(3)
		table := SimpleLookupTable(backends, 307)
		assertValidTable(t, table, 307, 3)
	})

	t.Run("distribution is round-robin modulo", func(t *testing.T) {
		backends := makeBackends(5)
		var m uint32 = 47
		table := SimpleLookupTable(backends, m)
		for i := range m {
			assert.Equal(t, i%5, table[i], "table[%d] must equal %d %% 5", i, i)
		}
	})

	t.Run("single backend fills entire table", func(t *testing.T) {
		backends := makeBackends(1)
		var m uint32 = 307
		table := SimpleLookupTable(backends, m)
		assertValidTable(t, table, m, 1)
		for i, v := range table {
			assert.Equal(t, uint32(0), v, "table[%d] must be 0 with single backend", i)
		}
	})

	t.Run("all backends appear in output", func(t *testing.T) {
		backends := makeBackends(7)
		var m uint32 = 307
		table := SimpleLookupTable(backends, m)
		assertValidTable(t, table, m, 7)
		indices := backendIndicesInTable(table)
		for i := range uint32(7) {
			_, ok := indices[i]
			assert.True(t, ok, "backend index %d must appear in table", i)
		}
	})

	t.Run("large prime 65497", func(t *testing.T) {
		backends := makeBackends(10)
		var m uint32 = 65497
		table := SimpleLookupTable(backends, m)
		assertValidTable(t, table, m, 10)
	})
}

// ---------------------------------------------------------------------------
// NaiveFibLookupTable
// ---------------------------------------------------------------------------

func TestNaiveFibLookupTable(t *testing.T) {
	t.Run("basic correctness with 3 backends and prime 307", func(t *testing.T) {
		backends := makeBackends(3)
		var m uint32 = 307
		table := NaiveFibLookupTable(backends, m)
		assertValidTable(t, table, m, 3)
	})

	t.Run("single backend fills entire table", func(t *testing.T) {
		backends := makeBackends(1)
		var m uint32 = 307
		table := NaiveFibLookupTable(backends, m)
		assertValidTable(t, table, m, 1)
		for i, v := range table {
			assert.Equal(t, uint32(0), v, "table[%d] must be 0 with single backend", i)
		}
	})

	t.Run("all backends appear with 5 backends", func(t *testing.T) {
		backends := makeBackends(5)
		var m uint32 = 307
		table := NaiveFibLookupTable(backends, m)
		assertValidTable(t, table, m, 5)
		indices := backendIndicesInTable(table)
		for i := range uint32(5) {
			_, ok := indices[i]
			assert.True(t, ok, "backend index %d must appear in table", i)
		}
	})

	t.Run("two backends", func(t *testing.T) {
		backends := makeBackends(2)
		var m uint32 = 13
		table := NaiveFibLookupTable(backends, m)
		assertValidTable(t, table, m, 2)
	})

	t.Run("deterministic output", func(t *testing.T) {
		backends := makeBackends(3)
		var m uint32 = 47
		table1 := NaiveFibLookupTable(backends, m)
		table2 := NaiveFibLookupTable(backends, m)
		assert.Equal(t, table1, table2, "same input must produce same output")
	})

	t.Run("large prime 4071", func(t *testing.T) {
		backends := makeBackends(10)
		var m uint32 = 4071
		table := NaiveFibLookupTable(backends, m)
		assertValidTable(t, table, m, 10)
	})
}

// ---------------------------------------------------------------------------
// RobustSimpleLookupTable
// ---------------------------------------------------------------------------

func TestRobustSimpleLookupTable(t *testing.T) {
	t.Run("basic correctness with 3 backends and prime 307", func(t *testing.T) {
		backends := makeBackends(3)
		var m uint32 = 307
		table := RobustSimpleLookupTable(backends, m)
		assertValidTable(t, table, m, 3)
	})

	t.Run("single backend fills entire table", func(t *testing.T) {
		backends := makeBackends(1)
		var m uint32 = 307
		table := RobustSimpleLookupTable(backends, m)
		assertValidTable(t, table, m, 1)
		for i, v := range table {
			assert.Equal(t, uint32(0), v, "table[%d] must be 0 with single backend", i)
		}
	})

	t.Run("all backends appear with 7 backends", func(t *testing.T) {
		backends := makeBackends(7)
		var m uint32 = 307
		table := RobustSimpleLookupTable(backends, m)
		assertValidTable(t, table, m, 7)
		indices := backendIndicesInTable(table)
		for i := range uint32(7) {
			_, ok := indices[i]
			assert.True(t, ok, "backend index %d must appear in table", i)
		}
	})

	t.Run("simulates failed backend by excluding it from input", func(t *testing.T) {
		all := makeBackends(5)
		var m uint32 = 307

		// Simulate backend 2 failing: pass only backends 0,1,3,4.
		surviving := []*types.Backend{all[0], all[1], all[3], all[4]}
		table := RobustSimpleLookupTable(surviving, m)
		assertValidTable(t, table, m, 4)
	})

	t.Run("deterministic output", func(t *testing.T) {
		backends := makeBackends(3)
		var m uint32 = 47
		table1 := RobustSimpleLookupTable(backends, m)
		table2 := RobustSimpleLookupTable(backends, m)
		assert.Equal(t, table1, table2, "same input must produce same output")
	})

	t.Run("two backends with prime 13", func(t *testing.T) {
		backends := makeBackends(2)
		var m uint32 = 13
		table := RobustSimpleLookupTable(backends, m)
		assertValidTable(t, table, m, 2)
	})

	t.Run("large prime 4071 with 10 backends", func(t *testing.T) {
		backends := makeBackends(10)
		var m uint32 = 4071
		table := RobustSimpleLookupTable(backends, m)
		assertValidTable(t, table, m, 10)
	})
}

// ---------------------------------------------------------------------------
// RobustFibLookupTable
// ---------------------------------------------------------------------------

func TestRobustFibLookupTable(t *testing.T) {
	t.Run("basic correctness with 3 backends and prime 307", func(t *testing.T) {
		backends := makeBackends(3)
		var m uint32 = 307
		table := RobustFibLookupTable(backends, m)
		assertValidTable(t, table, m, 3)
	})

	t.Run("single backend fills entire table", func(t *testing.T) {
		backends := makeBackends(1)
		var m uint32 = 307
		table := RobustFibLookupTable(backends, m)
		assertValidTable(t, table, m, 1)
		for i, v := range table {
			assert.Equal(t, uint32(0), v, "table[%d] must be 0 with single backend", i)
		}
	})

	t.Run("all backends appear with 5 backends", func(t *testing.T) {
		backends := makeBackends(5)
		var m uint32 = 307
		table := RobustFibLookupTable(backends, m)
		assertValidTable(t, table, m, 5)
		indices := backendIndicesInTable(table)
		for i := range uint32(5) {
			_, ok := indices[i]
			assert.True(t, ok, "backend index %d must appear in table", i)
		}
	})

	t.Run("simulates failed backend by excluding it from input", func(t *testing.T) {
		all := makeBackends(5)
		var m uint32 = 307

		// Simulate backend 2 failing: pass only backends 0,1,3,4.
		surviving := []*types.Backend{all[0], all[1], all[3], all[4]}
		table := RobustFibLookupTable(surviving, m)
		assertValidTable(t, table, m, 4)
	})

	t.Run("deterministic output", func(t *testing.T) {
		backends := makeBackends(3)
		var m uint32 = 47
		table1 := RobustFibLookupTable(backends, m)
		table2 := RobustFibLookupTable(backends, m)
		assert.Equal(t, table1, table2, "same input must produce same output")
	})

	t.Run("two backends with prime 13", func(t *testing.T) {
		backends := makeBackends(2)
		var m uint32 = 13
		table := RobustFibLookupTable(backends, m)
		assertValidTable(t, table, m, 2)
	})

	t.Run("large prime 4071 with 10 backends", func(t *testing.T) {
		backends := makeBackends(10)
		var m uint32 = 4071
		table := RobustFibLookupTable(backends, m)
		assertValidTable(t, table, m, 10)
	})
}

// ---------------------------------------------------------------------------
// ReverseCoordinatesLookupTable
// ---------------------------------------------------------------------------

func TestReverseCoordinatesLookupTable(t *testing.T) {
	t.Run("basic correctness with 3 backends and prime 307", func(t *testing.T) {
		backends := makeBackends(3)
		var m uint32 = 307
		table := ReverseCoordinatesLookupTable(backends, m)
		assertValidTable(t, table, m, 3)
	})

	t.Run("single backend fills entire table", func(t *testing.T) {
		backends := makeBackends(1)
		var m uint32 = 307
		table := ReverseCoordinatesLookupTable(backends, m)
		assertValidTable(t, table, m, 1)
		for i, v := range table {
			assert.Equal(t, uint32(0), v, "table[%d] must be 0 with single backend", i)
		}
	})

	t.Run("all backends appear with 5 backends", func(t *testing.T) {
		backends := makeBackends(5)
		var m uint32 = 307
		table := ReverseCoordinatesLookupTable(backends, m)
		assertValidTable(t, table, m, 5)
		indices := backendIndicesInTable(table)
		for i := range uint32(5) {
			_, ok := indices[i]
			assert.True(t, ok, "backend index %d must appear in table", i)
		}
	})

	t.Run("simulates failed backend by excluding it from input", func(t *testing.T) {
		all := makeBackends(5)
		var m uint32 = 307

		// Simulate backend 2 failing: pass only backends 0,1,3,4.
		surviving := []*types.Backend{all[0], all[1], all[3], all[4]}
		table := ReverseCoordinatesLookupTable(surviving, m)
		assertValidTable(t, table, m, 4)
	})

	t.Run("consistent valid output across calls", func(t *testing.T) {
		// ReverseCoordinatesLookupTable uses map iteration (non-deterministic
		// order in Go) for filling remaining slots. We verify structural
		// properties hold across multiple calls instead of exact equality.
		backends := makeBackends(3)
		var m uint32 = 47
		for range 5 {
			table := ReverseCoordinatesLookupTable(backends, m)
			assertValidTable(t, table, m, 3)
			indices := backendIndicesInTable(table)
			for i := range uint32(3) {
				_, ok := indices[i]
				assert.True(t, ok, "backend %d must appear", i)
			}
		}
	})

	t.Run("two backends with prime 23", func(t *testing.T) {
		backends := makeBackends(2)
		var m uint32 = 23
		table := ReverseCoordinatesLookupTable(backends, m)
		assertValidTable(t, table, m, 2)
	})

	t.Run("many backends with large prime", func(t *testing.T) {
		backends := makeBackends(10)
		var m uint32 = 4071
		table := ReverseCoordinatesLookupTable(backends, m)
		assertValidTable(t, table, m, 10)
	})
}

// ---------------------------------------------------------------------------
// ShardedLookupTable
// ---------------------------------------------------------------------------

func TestShardedLookupTable(t *testing.T) {
	t.Run("panics with unimplemented", func(t *testing.T) {
		backends := makeBackendsNonPointer(3)
		assert.Panics(t, func() {
			ShardedLookupTable(backends, 307)
		}, "ShardedLookupTable must panic with 'unimplemented'")
	})
}

// ---------------------------------------------------------------------------
// nextPrime (unexported helper)
// ---------------------------------------------------------------------------

func TestNextPrime(t *testing.T) {
	t.Run("returns largest prime smaller than current", func(t *testing.T) {
		p, ok := nextPrime(100)
		require.True(t, ok)
		assert.Equal(t, uint32(89), p)
	})

	t.Run("returns false when no smaller prime exists", func(t *testing.T) {
		_, ok := nextPrime(2)
		assert.False(t, ok)
	})

	t.Run("returns false for 1", func(t *testing.T) {
		_, ok := nextPrime(1)
		assert.False(t, ok)
	})

	t.Run("walks down the prime list", func(t *testing.T) {
		p, ok := nextPrime(50000)
		require.True(t, ok)
		assert.Equal(t, uint32(44497), p)
	})

	t.Run("returns 2 when current is 3", func(t *testing.T) {
		p, ok := nextPrime(3)
		require.True(t, ok)
		assert.Equal(t, uint32(2), p)
	})
}

// ---------------------------------------------------------------------------
// isNotFullyDistributed (unexported helper)
// ---------------------------------------------------------------------------

func TestIsNotFullyDistributed(t *testing.T) {
	t.Run("returns false when all zero", func(t *testing.T) {
		d := map[uint32]uint32{0: 0, 1: 0, 2: 0}
		assert.False(t, isNotFullyDistributed(d))
	})

	t.Run("returns true when one entry is nonzero", func(t *testing.T) {
		d := map[uint32]uint32{0: 0, 1: 1, 2: 0}
		assert.True(t, isNotFullyDistributed(d))
	})

	t.Run("returns true when all nonzero", func(t *testing.T) {
		d := map[uint32]uint32{0: 5, 1: 3, 2: 7}
		assert.True(t, isNotFullyDistributed(d))
	})

	t.Run("empty map returns false", func(t *testing.T) {
		d := map[uint32]uint32{}
		assert.False(t, isNotFullyDistributed(d))
	})
}

// ---------------------------------------------------------------------------
// Prime constants
// ---------------------------------------------------------------------------

func TestPrimeConstants(t *testing.T) {
	assert.Equal(t, Prime(307), Prime307)
	assert.Equal(t, Prime(4071), Prime4071)
	assert.Equal(t, Prime(65497), Prime65497)
}

// ---------------------------------------------------------------------------
// Edge-case tests targeting uncovered branches
// ---------------------------------------------------------------------------

func TestReverseCoordinatesLookupTable_distributionExhausted(t *testing.T) {
	// Use a small m relative to the number of backends so each backend
	// gets a tiny distribution quota (m/n). This triggers the
	// "distribution[i] < 1" continue branch during coordinate iteration.
	backends := makeBackends(10)
	var m uint32 = 23
	table := ReverseCoordinatesLookupTable(backends, m)
	assertValidTable(t, table, m, 10)
}

func TestNaiveFibLookupTable_fibContinueBranch(t *testing.T) {
	// Use a large m with few backends so the fib index advances past
	// fib[0]=1 and fib[1]=1 to fib[2]=2, hitting the "k < fib[j]"
	// continue branch where the algorithm assigns consecutive entries
	// to the same backend.
	backends := makeBackends(2)
	var m uint32 = 307
	table := NaiveFibLookupTable(backends, m)
	assertValidTable(t, table, m, 2)
}

func TestNaiveFibLookupTable_banListGoto(t *testing.T) {
	// Use m slightly larger than n so that distribution[j] = (m/n)+1
	// is small. Backends get banned quickly, triggering the banList
	// check and goto nextentry branch.
	backends := makeBackends(5)
	var m uint32 = 13
	table := NaiveFibLookupTable(backends, m)
	assertValidTable(t, table, m, 5)
}

func TestRobustFibLookupTable_distributionExhausted(t *testing.T) {
	// Use a small m with many backends so distribution[j] = m/n is
	// tiny. Backends exhaust their quota quickly, hitting the
	// "distribution[j] < 1" continue branch in the inner fib loop.
	backends := makeBackends(10)
	var m uint32 = 23
	table := RobustFibLookupTable(backends, m)
	assertValidTable(t, table, m, 10)
}

func TestRobustFibLookupTable_remainderFill(t *testing.T) {
	// Choose m and n such that m % n > 0 and the initial coordinate
	// mapping leaves the first n positions partially empty, triggering
	// the remainder fill section.
	// With m=11, n=3: rem=2, distribution per backend = 3 (total 9).
	// 2 remaining slots need to be filled by the remainder section.
	backends := makeBackends(3)
	var m uint32 = 11
	table := RobustFibLookupTable(backends, m)
	assertValidTable(t, table, m, 3)
}

func TestRobustSimpleLookupTable_remainderFill(t *testing.T) {
	// Same approach as RobustFibLookupTable: choose m and n with
	// m % n > 0 to trigger the remainder fill section.
	backends := makeBackends(3)
	var m uint32 = 11
	table := RobustSimpleLookupTable(backends, m)
	assertValidTable(t, table, m, 3)
}
