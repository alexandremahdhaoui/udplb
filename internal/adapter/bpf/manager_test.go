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
package bpfadapter_test

import (
	"net"
	"testing"

	bpfadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/bpf"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"

	"github.com/alexandremahdhaoui/ebpfstruct/pkg/fakebpfstruct"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: implement unit tests

func TestDataStructureManager(t *testing.T) {
	var (
		mgr bpfadapter.DataStructureManager

		backendList    *fakebpfstruct.Array[*bpfadapter.BackendSpec]
		lookupTable    *fakebpfstruct.Array[uint32]
		assignmentFifo *fakebpfstruct.FIFO[bpfadapter.Assignment]
		sessionMap     *fakebpfstruct.Map[uuid.UUID, uint32]

		err error
	)

	setup := func(t *testing.T) {
		t.Helper()

		backendList = fakebpfstruct.NewArray[*bpfadapter.BackendSpec]()
		lookupTable = fakebpfstruct.NewArray[uint32]()
		assignmentFifo = fakebpfstruct.NewFIFO[bpfadapter.Assignment]()
		sessionMap = fakebpfstruct.NewMap[uuid.UUID, uint32]()

		mgr, err = bpfadapter.NewDataStructureManager("unit-test", bpfadapter.Objects{
			BackendList:    backendList,
			LookupTable:    lookupTable,
			AssignmentFIFO: assignmentFifo,
			SessionMap:     sessionMap,
		})
		require.NoError(t, err)
	}

	t.Run("Close", func(t *testing.T) {
		setup(t)
		assert.NoError(t, mgr.Close())
	})

	t.Run("Done", func(t *testing.T) {
		setup(t)
		assert.NoError(t, mgr.Close())
		<-mgr.Done()
	})

	t.Run("AssignmentSubscribe", func(t *testing.T) {
		setup(t)
		assignmentFifo.EXPECT("Subscribe", nil)

		n := 10
		expected := make([]bpfadapter.Assignment, n)
		for i := range n {
			expected[i] = bpfadapter.Assignment{
				SessionId: uuid.New(),
				BackendId: uuid.New(),
			}
		}

		go func() {
			for i := range n {
				assignmentFifo.Chan <- expected[i]
			}
			close(assignmentFifo.Chan)
		}()

		ch, err := mgr.AssignmentSubscribe()
		assert.NoError(t, err)
		i := 0
		for actual := range ch {
			assert.Equal(t, expected[i], actual)
			i++
		}
	})

	t.Run("SetObjects", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			setup(t)

			backendList.EXPECT("SetAndDeferSwitchover", nil)
			lookupTable.EXPECT("SetAndDeferSwitchover", nil)
			sessionMap.EXPECT("SetAndDeferSwitchover", nil)

			mac, err := net.ParseMAC("3e:8e:27:61:d6:1a")
			assert.NoError(t, err)

			expectedBL := []types.Backend{
				{
					Id: uuid.New(),
					Spec: types.BackendSpec{
						IP:      net.ParseIP("10.0.0.1"),
						Port:    12345,
						MacAddr: mac,
					},
				},
				{
					Id: uuid.New(),
					Spec: types.BackendSpec{
						IP:      net.ParseIP("10.0.0.2"),
						Port:    23456,
						MacAddr: mac,
					},
				},
			}

			expectedLup := []uint32{0, 1, 0, 0, 1, 1}
			expectedSess := map[uuid.UUID]uint32{
				uuid.New(): 0,
				uuid.New(): 1,
			}

			// -- must not return err
			assert.NoError(t, mgr.SetObjects(expectedBL, expectedLup, expectedSess))

			// -- active data structures must be correct.

			actualBL := backendList.GetActiveArray()
			actualLup := lookupTable.GetActiveArray()
			actualSess := sessionMap.GetActiveMap()

			// -- verify each backend
			assert.Equal(t, len(expectedBL), len(actualBL))
			for i := range len(expectedBL) {
				expectedIP := util.NetIPv4ToUint32(expectedBL[i].Spec.IP)
				expectedMac, err := util.ParseIEEE802MAC(expectedBL[i].Spec.MacAddr.String())
				assert.NoError(t, err)

				assert.Equal(t, expectedBL[i].Id[:], actualBL[i].Id[:])
				assert.Equal(t, expectedIP, actualBL[i].Ip)
				assert.Equal(t, expectedMac, actualBL[i].Mac)
				assert.Equal(t, uint16(expectedBL[i].Spec.Port), actualBL[i].Port)
			}

			// -- verify lup
			assert.Equal(t, expectedLup, actualLup)
			// -- verify sessions
			assert.Equal(t, expectedSess, actualSess)
		})
	})

	t.Run("SessionBatchUpdate", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			setup(t)
			sessionMap.EXPECT("BatchUpdate", nil)

			in := map[uuid.UUID]uint32{
				uuid.New(): 1,
				uuid.New(): 0,
			}

			assert.NoError(t, mgr.SessionBatchUpdate(in))
			assert.Equal(t, in, sessionMap.GetActiveMap())
		})
	})

	t.Run("SessionBatchDelete", func(t *testing.T) {
		setup(t)
		sessionMap.EXPECT("BatchDelete", nil)

		keyToDelete := uuid.New()
		otherKey := uuid.New()

		sessionMap.GetActiveMap()[keyToDelete] = 0
		sessionMap.GetActiveMap()[otherKey] = 1

		assert.NoError(t, mgr.SessionBatchDelete(keyToDelete))
		assert.Equal(t, map[uuid.UUID]uint32{otherKey: 1}, sessionMap.GetActiveMap())
	})
}
