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

package commonv1alpha1

import (
	"errors"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"

	"github.com/google/uuid"
)

// -----------------------------------------------------
// -- REQUEST METADATA
// -----------------------------------------------------

var (
	errInvalidRequestMetadata  = errors.New("invalid RequestMetadata")
	errATraceIdMustBeSpecified = errors.New("a traceId must be specified")
)

func (m *RequestMetadata) Validate() error {
	if _, err := uuid.Parse(m.RequestId); err != nil {
		return flaterrors.Join(err, errInvalidRequestMetadata)
	}

	if m.TraceId == "" {
		return flaterrors.Join(errATraceIdMustBeSpecified, errInvalidRequestMetadata)
	}

	return nil
}

// -----------------------------------------------------
// -- RESPONSE METADATA
// -----------------------------------------------------

var errInvalidResponseMetadata = errors.New("invalid ResponseMetadata")

func (m *ResponseMetadata) Validate() error {
	if m.TraceId == "" {
		return flaterrors.Join(errATraceIdMustBeSpecified, errInvalidResponseMetadata)
	}

	return nil
}

// -----------------------------------------------------
// -- ERROR
// -----------------------------------------------------

var (
	errInvalidDistributedError = errors.New("invalid DistributedError")
	errAnErrorMustBeSpecified  = errors.New("an error must be specified")
	errUnknownErrorType        = errors.New("unknown error type")
)

func (e *Error) Validate() error {
	if len(e.Errors) == 0 {
		return flaterrors.Join(errAnErrorMustBeSpecified, errInvalidDistributedError)
	}

	for _, derr := range e.Errors {
		if derr.Type == 0 {
			return flaterrors.Join(errUnknownErrorType, errInvalidDistributedError)
		}

		if derr.Error == "" {
			return flaterrors.Join(errAnErrorMustBeSpecified, errInvalidDistributedError)
		}
	}

	if err := e.Metadata.Validate(); err != nil {
		return flaterrors.Join(err, errInvalidDistributedError)
	}

	return nil
}
