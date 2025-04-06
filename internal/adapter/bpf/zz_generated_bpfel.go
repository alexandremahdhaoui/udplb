// Code generated by bpf2go; DO NOT EDIT.
//go:build (386 || amd64 || arm || arm64 || loong64 || mips64le || mipsle || ppc64le || riscv64) && linux

package bpfadapter

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"

	"github.com/cilium/ebpf"
)

type udplbAssignmentT struct {
	SessionId [16]byte /* uint128 */
	BackendId [16]byte /* uint128 */
}

type udplbBackendSpecT struct {
	Id   [16]byte /* uint128 */
	Ip   uint32
	Port uint16
	Mac  [6]uint8
	_    [4]byte
}

type udplbConfigT struct {
	Ip              uint32
	Port            uint16
	_               [2]byte
	LookupTableSize uint32
}

// loadUdplb returns the embedded CollectionSpec for udplb.
func loadUdplb() (*ebpf.CollectionSpec, error) {
	reader := bytes.NewReader(_UdplbBytes)
	spec, err := ebpf.LoadCollectionSpecFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("can't load udplb: %w", err)
	}

	return spec, err
}

// loadUdplbObjects loads udplb and converts it into a struct.
//
// The following types are suitable as obj argument:
//
//	*udplbObjects
//	*udplbPrograms
//	*udplbMaps
//
// See ebpf.CollectionSpec.LoadAndAssign documentation for details.
func loadUdplbObjects(obj interface{}, opts *ebpf.CollectionOptions) error {
	spec, err := loadUdplb()
	if err != nil {
		return err
	}

	return spec.LoadAndAssign(obj, opts)
}

// udplbSpecs contains maps and programs before they are loaded into the kernel.
//
// It can be passed ebpf.CollectionSpec.Assign.
type udplbSpecs struct {
	udplbProgramSpecs
	udplbMapSpecs
	udplbVariableSpecs
}

// udplbProgramSpecs contains programs before they are loaded into the kernel.
//
// It can be passed ebpf.CollectionSpec.Assign.
type udplbProgramSpecs struct {
	Udplb *ebpf.ProgramSpec `ebpf:"udplb"`
}

// udplbMapSpecs contains maps before they are loaded into the kernel.
//
// It can be passed ebpf.CollectionSpec.Assign.
type udplbMapSpecs struct {
	AssignmentFifo *ebpf.MapSpec `ebpf:"assignment_fifo"`
	BackendListA   *ebpf.MapSpec `ebpf:"backend_list_a"`
	BackendListB   *ebpf.MapSpec `ebpf:"backend_list_b"`
	LookupTableA   *ebpf.MapSpec `ebpf:"lookup_table_a"`
	LookupTableB   *ebpf.MapSpec `ebpf:"lookup_table_b"`
	SessionMapA    *ebpf.MapSpec `ebpf:"session_map_a"`
	SessionMapB    *ebpf.MapSpec `ebpf:"session_map_b"`
}

// udplbVariableSpecs contains global variables before they are loaded into the kernel.
//
// It can be passed ebpf.CollectionSpec.Assign.
type udplbVariableSpecs struct {
	ActivePointer    *ebpf.VariableSpec `ebpf:"active_pointer"`
	BackendListA_len *ebpf.VariableSpec `ebpf:"backend_list_a_len"`
	BackendListB_len *ebpf.VariableSpec `ebpf:"backend_list_b_len"`
	Config           *ebpf.VariableSpec `ebpf:"config"`
	LookupTableA_len *ebpf.VariableSpec `ebpf:"lookup_table_a_len"`
	LookupTableB_len *ebpf.VariableSpec `ebpf:"lookup_table_b_len"`
	SessionMapA_len  *ebpf.VariableSpec `ebpf:"session_map_a_len"`
	SessionMapB_len  *ebpf.VariableSpec `ebpf:"session_map_b_len"`
}

// udplbObjects contains all objects after they have been loaded into the kernel.
//
// It can be passed to loadUdplbObjects or ebpf.CollectionSpec.LoadAndAssign.
type udplbObjects struct {
	udplbPrograms
	udplbMaps
	udplbVariables
}

func (o *udplbObjects) Close() error {
	return _UdplbClose(
		&o.udplbPrograms,
		&o.udplbMaps,
	)
}

// udplbMaps contains all maps after they have been loaded into the kernel.
//
// It can be passed to loadUdplbObjects or ebpf.CollectionSpec.LoadAndAssign.
type udplbMaps struct {
	AssignmentFifo *ebpf.Map `ebpf:"assignment_fifo"`
	BackendListA   *ebpf.Map `ebpf:"backend_list_a"`
	BackendListB   *ebpf.Map `ebpf:"backend_list_b"`
	LookupTableA   *ebpf.Map `ebpf:"lookup_table_a"`
	LookupTableB   *ebpf.Map `ebpf:"lookup_table_b"`
	SessionMapA    *ebpf.Map `ebpf:"session_map_a"`
	SessionMapB    *ebpf.Map `ebpf:"session_map_b"`
}

func (m *udplbMaps) Close() error {
	return _UdplbClose(
		m.AssignmentFifo,
		m.BackendListA,
		m.BackendListB,
		m.LookupTableA,
		m.LookupTableB,
		m.SessionMapA,
		m.SessionMapB,
	)
}

// udplbVariables contains all global variables after they have been loaded into the kernel.
//
// It can be passed to loadUdplbObjects or ebpf.CollectionSpec.LoadAndAssign.
type udplbVariables struct {
	ActivePointer    *ebpf.Variable `ebpf:"active_pointer"`
	BackendListA_len *ebpf.Variable `ebpf:"backend_list_a_len"`
	BackendListB_len *ebpf.Variable `ebpf:"backend_list_b_len"`
	Config           *ebpf.Variable `ebpf:"config"`
	LookupTableA_len *ebpf.Variable `ebpf:"lookup_table_a_len"`
	LookupTableB_len *ebpf.Variable `ebpf:"lookup_table_b_len"`
	SessionMapA_len  *ebpf.Variable `ebpf:"session_map_a_len"`
	SessionMapB_len  *ebpf.Variable `ebpf:"session_map_b_len"`
}

// udplbPrograms contains all programs after they have been loaded into the kernel.
//
// It can be passed to loadUdplbObjects or ebpf.CollectionSpec.LoadAndAssign.
type udplbPrograms struct {
	Udplb *ebpf.Program `ebpf:"udplb"`
}

func (p *udplbPrograms) Close() error {
	return _UdplbClose(
		p.Udplb,
	)
}

func _UdplbClose(closers ...io.Closer) error {
	for _, closer := range closers {
		if err := closer.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Do not access this directly.
//
//go:embed zz_generated_bpfel.o
var _UdplbBytes []byte
