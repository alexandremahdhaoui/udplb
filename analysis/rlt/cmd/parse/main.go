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
package main

import (
	"fmt"
	"os"
	"regexp"
)

const usage = `USAGE:
	%s <BENCHMARK LOG PATH>
`

// BenchmarkLookupTable/func=RobustSimple/prime=65497/nBefore=255/nAfter=299-16          10         298616768 ns/op                59.91 %unchangedEntries/op      16534052 B/op     132515 allocs/op

const (
	bmLupTableRegex = `BenchmarkLookupTable`
	slashDelimiter  = `/`
	funcRegex       = `func=([a-zA-Z]+)`
	primeRegex      = `prime=(\d+)`
	nBeforeRegex    = `nBefore=(\d+)`
	nAfterRegex     = `nAfter=(\d+)`

	spaceTabDelimiter = `[\ \t]+`
	numbersRegex      = `\d+`

	execTimeRegex         = `(\d+) ns/op`
	unchangedEntriesRegex = `(\d+\.\d+) %unchangedEntries/op`
	allocatedBytesRegex   = `(\d+) B/op`
	allocationsRegex      = `(\d+) allocs/op`
)

var regex = regexp.MustCompile(
	bmLupTableRegex + slashDelimiter +
		funcRegex + slashDelimiter +
		primeRegex + slashDelimiter +
		nBeforeRegex + slashDelimiter +
		nAfterRegex + `-\d+` + spaceTabDelimiter +

		numbersRegex + spaceTabDelimiter +

		execTimeRegex + spaceTabDelimiter +
		unchangedEntriesRegex + spaceTabDelimiter +
		allocatedBytesRegex + spaceTabDelimiter +
		allocationsRegex,
)

func main() {
	if len(os.Args) != 2 {
		fmtExit(usage, os.Args[0])
	}

	logPath := os.Args[1]
	b, err := os.ReadFile(logPath)
	if err != nil {
		fmtExit("error reading benchmark log file: %s", logPath)
	}

	match := regex.FindAllSubmatch(b, -1)
	if match == nil {
		fmtExit("cannot match any records\n")
	}

	fmt.Println(
		"algorithm,prime,nBefore,nAfter,execTime,unchangedEntries,allocatedBytes,allocations",
	)
	for _, m := range match {
		fmt.Printf("%s,%s,%s,%s,%s,%s,%s,%s\n", m[1], m[2], m[3], m[4], m[5], m[6], m[7], m[8])
	}
}

func fmtExit(format string, a ...any) {
	fmt.Printf(format, a...)
	os.Exit(1)
}
