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

import (
	"errors"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/cilium/ebpf"
)

var (
	ErrNewBPFVariable          = errors.New("cannot create new BPFVariable")
	ErrEBPFObjectsMustNotBeNil = errors.New("ebpf objects must not be nil")

	ErrCannotCreateObjectsFromUnknownUDPLBImplementation = errors.New(
		"cannot create bpf.Objects struct from unknown bpf.UDPLB implementation",
	)
)

// -------------------------------------------------------------------
// -- BPF VARIABLE
// -------------------------------------------------------------------

// This is only used in order to write tests. No fancy feature about it.
// Maybe in the future add support for:
//   - Get()
//   - caching & no-cache for Get().
//   - Set with differable switchover in order to sync switchover with
//     many bpf data structures.
type Variable[T any] interface {
	// Set the variable.
	Set(v T) error
}

type bpfVariable[T any] struct {
	obj *ebpf.Variable
}

// Set implements BPFVariable.
func (b *bpfVariable[T]) Set(v T) error {
	return b.obj.Set(v)
}

func NewVariable[T any](obj *ebpf.Variable) (Variable[T], error) {
	if obj == nil {
		return nil, flaterrors.Join(ErrEBPFObjectsMustNotBeNil, ErrNewBPFVariable)
	}

	return &bpfVariable[T]{obj: obj}, nil
}
