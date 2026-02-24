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
package util

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// AnyPtrIsNil
// ---------------------------------------------------------------------------

func TestAnyPtrIsNil_AllNonNil(t *testing.T) {
	a, b, c := 1, "hello", 3.14
	assert.False(t, AnyPtrIsNil(&a, &b, &c))
}

func TestAnyPtrIsNil_OneNil(t *testing.T) {
	a := 1
	assert.True(t, AnyPtrIsNil(&a, nil))
}

func TestAnyPtrIsNil_AllNil(t *testing.T) {
	assert.True(t, AnyPtrIsNil(nil, nil, nil))
}

func TestAnyPtrIsNil_NoArgs(t *testing.T) {
	assert.False(t, AnyPtrIsNil())
}

func TestAnyPtrIsNil_SingleNonNil(t *testing.T) {
	v := 42
	assert.False(t, AnyPtrIsNil(&v))
}

func TestAnyPtrIsNil_SingleNil(t *testing.T) {
	assert.True(t, AnyPtrIsNil(nil))
}

// ---------------------------------------------------------------------------
// IgnoreErr
// ---------------------------------------------------------------------------

func TestIgnoreErr_ReturnsData(t *testing.T) {
	result := IgnoreErr(42, nil)
	assert.Equal(t, 42, result)
}

func TestIgnoreErr_IgnoresError(t *testing.T) {
	result := IgnoreErr("value", errors.New("some error"))
	assert.Equal(t, "value", result)
}

func TestIgnoreErr_ZeroValueWithError(t *testing.T) {
	result := IgnoreErr(0, errors.New("some error"))
	assert.Equal(t, 0, result)
}
