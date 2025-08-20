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
package dvds_test

import (
	"context"
	"testing"
	"time"

	dvdscontroller "github.com/alexandremahdhaoui/udplb/internal/controller/dvds"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/alexandremahdhaoui/udplb/internal/util/mocks/mocktypes"

	"github.com/stretchr/testify/assert"
)

func TestDVDS(t *testing.T) {
	type (
		T = int
		U = map[T]string
	)

	var (
		ctx context.Context

		dvds types.DVDS[T, U]

		stm        types.StateMachine[T, U]
		wal        types.WAL[T]
		watcherMux *util.WatcherMux[U]
	)

	setup := func(t *testing.T) {
		t.Helper()

		ctx = context.Background()
		stm = mocktypes.NewMockStateMachine[T, U](t)
		wal = mocktypes.NewMockWAL[T](t)
		watcherMux = util.NewWatcherMux(5, util.NewDispatchFuncWithTimeout[U](1*time.Second))

		dvds = dvdscontroller.New(
			stm,
			wal,
			watcherMux,
		)
	}

	t.Run("TODO", func(t *testing.T) {
		setup(t)
		err := dvds.Run(ctx)
		assert.NoError(t, err)
	})
}
