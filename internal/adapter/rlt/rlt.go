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
	"github.com/alexandremahdhaoui/udplb/internal/types"
)

type Prime uint32

const (
	Prime307   Prime = 307   // Recommended if n_backends < 3
	Prime4071  Prime = 4071  // Recommended if n_backends < 40
	Prime65497 Prime = 65497 // Recommended if n_backends < 650
)

var (
	primes = []uint32{
		2, 3, 5, 7, 13, 17, 19, 31, 61, 89, 107, 127, 521, 607,
		1279, 2203, 2281, 3217, 4253, 4423, 9689, 9941, 11213,
		19937, 21701, 23209, 44497, // mersenne prime exponents.
	}

	fib = []uint32{
		1, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233, 377, 610,
		987, 1587, 2584, 4181,
	}
)

const nCoordinates = types.NCoordinates

// Let p a Prime equal to the lookupTableLength.
//   - Use backend uuid, split it by x, e.g. with x=4: 4 * __u32.
//   - mod[i] = uint32(uuid[i*32, (i+1)*32]) % p. i in [0,x]
//   - coordinates = [mod[0], mod[1], ... mod[x]]
//
// Now we compute the lookup table in reverse. From the user POV.
//   - Make a list of y prime numbers in the range of [x, p] in descending order.
//   - For each primes compute "mod[i][y]" where we replace p with
//     the prime at index y.
//   - For each backend set the index mod[i][y] in the lookup table
//     to this backend.
//     => Also: y(n) > 2 * y(n+1). Or it could just be p/2???
//   - This allows us to fill all lookup table entries which index is a multiple
//     of the mod[i][y].
//
// Do this iteratively, until the lookup table is full.
// When y = y_list[len(y) - 1] then the lookup table is guaranted to be full.
//
// Test shows that ReverseCoordinatesLookupTable is resilient to backend failures.
// It is less performant on scale up.
func ReverseCoordinatesLookupTable(
	availableBackends []*types.Backend,
	// let m the length of the lookup table
	m uint32,
) []uint32 {
	// let `out` the lookup table of length m.
	out := make([]uint32, m)
	// let n the number of available backends.
	n := uint32(len(availableBackends))

	unset := make(map[uint32]struct{})
	for i := range m {
		unset[i] = struct{}{}
	}

	distribution := make(map[uint32]uint32, n)
	for i := range n {
		distribution[i] = m / n
	}

	coord := make([][nCoordinates]uint32, n)
	for i, b := range availableBackends {
		coord[i] = b.Coordinates()
	}

	prime := m // init prime as m.
	shouldContinue := true
	for shouldContinue {
		// -- for each backend.
		for i := range n {
			// for each coordinate of the backend.
			for j := range nCoordinates {
				if distribution[i] < 1 {
					continue
				}

				mod := coord[i][j] % prime
				// -- for each multiple of that prime.
				for k := range m / prime {
					idx := (k + 1) * mod
					if _, ok := unset[idx]; !ok {
						continue
					}

					out[idx] = i
					distribution[i] -= 1
					delete(unset, idx)
				}
			}
		}

		// -- pick a next prime prime.
		prime, shouldContinue = nextPrime(prime)
		shouldContinue = shouldContinue && isNotFullyDistributed(distribution)
	}

	// -- fill remaining
	var i uint32
	for k := range unset {
		if i > n-1 {
			i = 0
		}

		out[k] = i
		i++
	}

	return out
}

func nextPrime(current uint32) (uint32, bool) {
	// INFO: this could be O(1) but the prime list grow logarithmically.
	// The mersonne prime exponents are of the form (2^p)-1, hence this
	// list of primes grows logaritmically with respect to maximum lookup
	// table length.
	// Anyway this lookup table max value is constrained by the bpf program
	// to be at most `1 << 16 - 1` for now.
	// In conclusion, this func is about O(log(m_max)).
	for i := range len(primes) {
		j := (len(primes) - 1) - i
		if primes[j] < current {
			return primes[j], true
		}
	}
	return 0, false
}

func isNotFullyDistributed(distribution map[uint32]uint32) bool {
	for _, v := range distribution {
		if v > 0 {
			return true
		}
	}
	return false
}

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
func ShardedLookupTable(
	availableBackends []types.Backend,
	lookupTableLength uint32,
) []uint32 {
	panic("unimplemented")
}

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
	availableBackends []*types.Backend,
	// let m the length of the lookup table
	m uint32,
) []uint32 {
	// let n the number of available backends.
	n := uint32(len(availableBackends))
	// let rem the remainder of m/n.
	rem := m % n

	// evenly distribute map.
	distribution := make(map[uint32]uint32, n)
	banned := make(map[uint32]struct{}, n)
	for j := range n {
		distribution[j] = m / n
	}

	// lookup table
	lup := make(map[uint32]uint32, m)

	// M[j] holds the latest index of B[j]
	M := make([]uint32, n)
	for j := range n {
		mod := availableBackends[j].Coordinates()[0] % m
		M[j] = mod

		// fill this entry.
		lup[mod] = j
		distribution[j] -= 1
	}

	for i := range len(fib) {
		for j := range n {
			if uint32(len(banned)) == n {
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
				// -- Persist backend index in the lookup table.
				// j being the index of the current backend with respect to the provided list.
				lup[Mj] = j
				distribution[j] -= 1
			}
		}
	}

exitLoop:
	// TODO: this operation could be faster by caching the list of non assigned
	// keys.
	// -- fill reamining fields.
	var i, j uint32
	for i = range n {
		if j > rem-1 {
			break
		}

		if _, ok := lup[i]; ok {
			// until we find an empty slot.
			continue
		}

		// -- Persist backend index in the lookup table.
		lup[i] = j
		j++
	}

	// -- output format.
	out := make([]uint32, m)
	for k, v := range lup {
		out[k] = v
	}

	return out
}

func RobustSimpleLookupTable(
	availableBackends []*types.Backend,
	lookupTableLength uint32,
) []uint32 {
	// let n the number of available backends.
	n := uint32(len(availableBackends))
	// let m the length of the lookup table
	m := uint32(lookupTableLength)
	// let rem the remainder of m/n.
	rem := m % n

	// evenly distribute map.
	distribution := make(map[uint32]uint32, n)
	banned := make(map[uint32]struct{})
	for j := range n {
		distribution[j] = m / n
	}

	// lookup table
	lup := make(map[uint32]uint32, m)

	// M[j] holds the latest index of B[j]
	M := make([]uint32, n)
	for j := range n {
		mod := availableBackends[j].Coordinates()[0] % m
		M[j] = mod

		// fill this entry.
		lup[mod] = j
		distribution[j] -= 1
	}

	// populate lup
	for uint32(len(banned)) != n {
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
			lup[Mj] = j
			distribution[j] -= 1
		}
	}

	// fill reamining fields.
	var i, j uint32
	for i = range n {
		if j > rem-1 {
			break
		}

		if _, ok := lup[i]; ok {
			// until we find an empty slot.
			continue
		}

		lup[i] = j
		j++
	}

	// make into outputformat.
	out := make([]uint32, m)
	for k, v := range lup {
		out[k] = v
	}

	return out
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
	availableBackends []*types.Backend,
	lookupTableLength uint32,
) []uint32 {
	// TODO: improve this to make less disruption when a backend is down or a new
	// backend is added. The idea is to shard the lookup table, and assign each
	// backend to one to many shards deterministically. When a backend becomes down
	// the other backends will take over its shards, without changing previously
	// held shards.

	var n, m, j, k, currentEntry uint32

	n = uint32(len(availableBackends))
	m = uint32(lookupTableLength)

	j = 0 // fib index
	k = 0 // counter < fib[j]
	currentEntry = 0

	// evenly distribute map.
	banned := 0
	banList := make(map[uint32]struct{}, n)
	distribution := make(map[uint32]uint32, n)
	for i := range n {
		// Ensure there is always an unbanned entry in order to
		// fill the `m % n` remaining entries.
		distribution[i] = (m / n) + 1
	}

	out := make([]uint32, m)
	for i := range m {
		out[i] = currentEntry

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
		if j < uint32(len(fib)-1) {
			// reset fib index (j) if we exceed it.
			j = 0
		}

		if _, ok := banList[currentEntry]; ok {
			goto nextentry
		}
	}

	return out
}

func SimpleLookupTable(
	availableBackends []*types.Backend,
	lookupTableLength uint32,
) []uint32 {
	m := uint32(lookupTableLength)
	out := make([]uint32, m)

	for i := range m {
		out[i] = i % uint32(len(availableBackends))
	}

	return out
}
