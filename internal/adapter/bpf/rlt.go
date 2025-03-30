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
package bpfadapter

type Prime uint64

const (
	Prime307   Prime = 307   // Recommended if n_backends < 3
	Prime4071  Prime = 4071  // Recommended if n_backends < 40
	Prime65497 Prime = 65497 // Recommended if n_backends < 650
)

// Populates a lookup table using a determistic robust algorithm.
//
// In case the number of available backend is scaled up or decreased it will
// preserve the order of existing backend assignments.
// Hence it will let traffic unrelated to the scaled down nodes unaffected.
//
// The function must return a list of integer keys corresponding to the indices
// of the lookup table and a list of string values corresponding to the Ids of
// backends.
//
// NB: this is bad because the order of available backends might change when a
// backend becomes down or its replica number is scaled up.
//
// => Maglev is hashing the names of the backend. That might help because hashing
// the names will ensure a backend identified by its name/id hash will always end up
// in the same shard.
// What about collisions? Well we just need to pick a sufficiently large number of
// shards.
func ShardedLookupTable(availableBackends []Backend, lookupTableLength Prime) ([]int, []string) {
	panic("unimplemented")
}

var fib = []uint64{1, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233, 377, 610, 987, 1587, 2584, 4181}

// Populates a robust lookup table.
//
// Let `n` the number of backends.
// Let `m` the length of the lookup table, a relatively large Prime number.
// Let `B[j]` the j-th available Backend.
// Let `H[j]` the hash of the j-th available Backend.
// Let `M[j]` the result of the following expression: `Hj % m`
// Let `lup` the ident of the lookup table.
//
// Let `i` an index in range len(fib).
// Let `j` an index in range n.
// Let `k` an index in range fib[i].
//
// The algorithm will compute the lookup table by:
// - Initializing each Bi such that: lup[Hi] = Bi.Id
// - Using the NaiveFib algorithm to set the next available entry in the map.
func RobustFibLookupTable(
	availableBackends []*Backend,
	lookupTableLength Prime,
) (keys []uint64, values []string) {
	// let n the number of available backends.
	n := uint64(len(availableBackends))
	// let m the length of the lookup table
	m := uint64(lookupTableLength)
	// let rem the remainder of m/n.
	rem := m % n

	// evenly distribute map.
	distribution := make(map[uint64]uint64, n)
	banned := make(map[uint64]struct{}, n)
	for j := range n {
		distribution[j] = m / n
	}

	// lookup table
	lup := make(map[uint64]string, m)

	// M[j] holds the latest index of B[j]
	M := make([]uint64, n)
	for j := range n {
		mod := availableBackends[j].HashedId() % m
		M[j] = mod

		// fill this entry.
		lup[mod] = availableBackends[j].Id.String()
		distribution[j] -= 1
	}

	for i := range len(fib) {
		for j := range n {
			if uint64(len(banned)) == n {
				goto exitLoop
			}

			if distribution[j] < 1 {
				continue
			}

			for range fib[i] {
				if distribution[j] < 1 {
					// ensure the map is evenly distributed.
					banned[j] = struct{}{}
					break
				}

				Mj := M[j] + 1
				for { // until we find an empty slot.
					if Mj > m-1 {
						Mj = 0 // start again from 0 if we exceded lookup table's length.
					}

					if _, ok := lup[Mj]; !ok {
						break
					}

					Mj++
				}

				M[j] = Mj
				lup[Mj] = availableBackends[j].Id.String()
				distribution[j] -= 1
			}
		}
	}

exitLoop:
	// fill reamining fields.
	var i, j uint64
	for i = 0; i < n; i++ {
		if j > rem-1 {
			break
		}

		if _, ok := lup[i]; ok {
			// until we find an empty slot.
			continue
		}

		lup[i] = availableBackends[j].Id.String()
		j++
	}

	// make into outputformat.
	keys = make([]uint64, m)
	values = make([]string, m)
	for k, v := range lup {
		keys[k] = k
		values[k] = v
	}

	return keys, values
}

func RobustSimpleLookupTable(
	availableBackends []*Backend,
	lookupTableLength Prime,
) (keys []uint64, values []string) {
	// let n the number of available backends.
	n := uint64(len(availableBackends))
	// let m the length of the lookup table
	m := uint64(lookupTableLength)
	// let rem the remainder of m/n.
	rem := m % n

	// evenly distribute map.
	distribution := make(map[uint64]uint64, n)
	banned := make(map[uint64]struct{})
	for j := range n {
		distribution[j] = m / n
	}

	// lookup table
	lup := make(map[uint64]string, m)

	// M[j] holds the latest index of B[j]
	M := make([]uint64, n)
	for j := range n {
		mod := availableBackends[j].HashedId() % m
		M[j] = mod

		// fill this entry.
		lup[mod] = availableBackends[j].Id.String()
		distribution[j] -= 1
	}

	// populate lup
	for {
		if uint64(len(banned)) == n {
			break
		}

		for j := range n {
			if distribution[j] < 1 {
				// ensure the map is evenly distributed.
				banned[j] = struct{}{}
				continue
			}

			Mj := M[j] + 1

			for { // until we find an empty slot.
				if Mj > m-1 {
					Mj = 0 // start again from 0 if we exceded lookup table's length.
				}

				if _, ok := lup[Mj]; !ok {
					break
				}

				Mj++
			}

			M[j] = Mj
			lup[Mj] = availableBackends[j].Id.String()
			distribution[j] -= 1
		}
	}

	// fill reamining fields.
	var i, j uint64
	for i = 0; i < n; i++ {
		if j > rem-1 {
			break
		}

		if _, ok := lup[i]; ok {
			// until we find an empty slot.
			continue
		}

		lup[i] = availableBackends[j].Id.String()
		j++
	}

	// make into outputformat.
	keys = make([]uint64, m)
	values = make([]string, m)
	for k, v := range lup {
		keys[k] = k
		values[k] = v
	}

	return keys, values
}

// Populates a lookup table using fibonacci.
//
// This is a stupid algo. We populate the lookup table by looping over all entries
// and assigning `k=fib[j]` consecutive entries with each of the backends. When `k`
// entries was filled by each backend, we increment j.
// The algorithm also ensures the lookup table has been evenly distributed, by
// tracking the number of
//
// TODO: Implement the maglev lookup table algo instead.
// TODO: we must lock the lookup table while the table is being updated.
func NaiveFibLookupTable(
	availableBackends []*Backend,
	lookupTableLength Prime,
) (keys []uint64, values []string) {
	// TODO: improve this to make less disruption when a backend is down or a new
	// backend is added. The idea is to shard the lookup table, and assign each
	// backend to one to many shards deterministically. When a backend becomes down
	// the other backends will take over its shards, without changing previously
	// held shards.

	var n, m, j, k, currentEntry uint64

	n = uint64(len(availableBackends))
	m = uint64(lookupTableLength)

	j = 0 // fib index
	k = 0 // counter < fib[j]
	currentEntry = 0

	// evenly distribute map.
	banned := 0
	banList := make(map[uint64]struct{}, n)
	distribution := make(map[uint64]uint64, n)
	for i := range n {
		// Ensure there is always an unbanned entry in order to
		// fill the `m % n` remaining entries.
		distribution[i] = (m / n) + 1
	}

	keys = make([]uint64, m)
	values = make([]string, m)

	for i := range m {
		keys[i] = i
		values[i] = availableBackends[currentEntry].Id.String()

		// ensure the map is evenly distributed
		distribution[currentEntry] -= 1
		if distribution[currentEntry] < 1 {
			banned += 1
			banList[currentEntry] = struct{}{}
			goto nextentry
		}

		// update k's iteration.
		k += 1
		if k < fib[j] {
			continue
		}

	nextentry:
		// reset k and update the current entry.
		k = 0
		currentEntry += 1
		if currentEntry < n {
			continue
		}

		// reset currentEntry && update fib index (j).
		currentEntry = 0
		j += 1
		if j < uint64(len(fib)-1) {
			// reset fib index (j) if we exceed it.
			j = 0
		}

		if _, ok := banList[currentEntry]; ok {
			goto nextentry
		}
	}

	return keys, values
}

func SimpleLookupTable(
	availableBackends []*Backend,
	lookupTableLength Prime,
) (keys []uint64, values []string) {
	m := uint64(lookupTableLength)
	keys = make([]uint64, m)
	values = make([]string, m)

	for i := range m {
		keys[i] = i
		values[i] = availableBackends[i%uint64(len(availableBackends))].Id.String()
	}

	return keys, values
}
