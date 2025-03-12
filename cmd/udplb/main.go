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

// TODO: end to end test with qemu vms or so.
// TODO: init a context w/ ifname & process name & use slog.{Info,Error}Context instead.

func main() {}

//	if len(os.Args) != 2 {
//		fmt.Fprintf(os.Stderr, usage, os.Args[0])
//		os.Exit(1)
//	}
const usage = `USAGE:
	%s <config file path>
`
