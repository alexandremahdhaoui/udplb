package types_test

import (
	"net"
	"testing"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCoordinates(t *testing.T) {
	t.Run("should return the correct coordinates", func(t *testing.T) {
		id := uuid.New()
		spec := types.BackendSpec{
			IP:      net.ParseIP("192.168.1.1"),
			Port:    8080,
			MacAddr: func() net.HardwareAddr {
				mac, _ := net.ParseMAC("00:00:5e:00:53:01")
				return mac
			}(),
		}
		status := types.BackendStatus{}
		b := types.NewBackend(id, spec, status)

		assert.NotNil(t, b.Coordinates())
	})
}

func TestNewBackend(t *testing.T) {
	t.Run("should create a new backend", func(t *testing.T) {
		id := uuid.New()
		spec := types.BackendSpec{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 8080,
			MacAddr: func() net.HardwareAddr {
				mac, _ := net.ParseMAC("00:00:5e:00:53:01")
				return mac
			}(),
		}
		status := types.BackendStatus{}
		backend := types.NewBackend(id, spec, status)
		assert.NotNil(t, backend)
		assert.Equal(t, spec, backend.Spec)
		assert.Equal(t, status, backend.Status)
	})
}
