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

package eventv1alpha1

import (
	"errors"

	commonv1alpha1 "github.com/alexandremahdhaoui/udplb/pkg/apis/common/v1alpha1"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"

	"github.com/google/uuid"
)

// -----------------------------------------------------
// -- EVENT
// -----------------------------------------------------

var (
	errInvalidEvent         = errors.New("invalid Event")
	errAVerbMustBeSpecified = errors.New("a verb must be specified")
)

func (e *Event) Validate() error {
	if e.Verb == "" {
		return flaterrors.Join(errAVerbMustBeSpecified, errInvalidEvent)
	}

	for _, id := range []string{
		e.SessionId,
		e.SubjectId,
		e.ObjectId,
	} {
		if _, err := uuid.Parse(id); err != nil {
			return flaterrors.Join(err, errInvalidEvent)
		}
	}

	return nil
}

// -----------------------------------------------------
// -- EVENT REQUEST
// -----------------------------------------------------

var errInvalidEventRequest = errors.New("invalid EventRequest")

func (r *EventRequest) Validate() error {
	for _, v := range []commonv1alpha1.Validator{
		r.Metadata,
		r.AuthData,
		r.Event,
	} {
		if err := v.Validate(); err != nil {
			return flaterrors.Join(err, errInvalidEventRequest)
		}
	}

	return nil
}

// -----------------------------------------------------
// -- EVENT RESPONSE
// -----------------------------------------------------

var errInvalidEventResponse = errors.New("invalid EventResponse")

func (r *EventResponse) Validate() error {
	for _, v := range []commonv1alpha1.Validator{
		r.Metadata,
		r.Event,
	} {
		if err := v.Validate(); err != nil {
			return flaterrors.Join(err, errInvalidEventResponse)
		}
	}

	return nil
}
