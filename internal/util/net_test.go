package util_test

import (
	"net"
	"testing"

	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestParseIPV4ToUint32(t *testing.T) {
	t.Run("should parse a valid IPv4 address", func(t *testing.T) {
		ip := "192.168.1.1"
		expected := uint32(3232235777)
		actual, err := util.ParseIPV4ToUint32(ip)
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)
	})

	t.Run("should return an error for an invalid IPv4 address", func(t *testing.T) {
		ip := "not an ip"
		_, err := util.ParseIPV4ToUint32(ip)
		assert.Error(t, err)
	})

	t.Run("should return an error for an IPv6 address", func(t *testing.T) {
		ip := "::1"
		_, err := util.ParseIPV4ToUint32(ip)
		assert.Error(t, err)
	})
}

func TestNetIPv4ToUint32(t *testing.T) {
	t.Run("should convert a valid net.IP to uint32", func(t *testing.T) {
		ip := net.ParseIP("192.168.1.1")
		expected := uint32(3232235777)
		actual, err := util.NetIPv4ToUint32(ip)
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)
	})

	t.Run("should return an error for a nil IP", func(t *testing.T) {
		var ip net.IP
		_, err := util.NetIPv4ToUint32(ip)
		assert.Error(t, err)
	})

	t.Run("should return an error for an IPv6 address", func(t *testing.T) {
		ip := net.ParseIP("::1")
		_, err := util.NetIPv4ToUint32(ip)
		assert.Error(t, err)
	})
}

func TestParseIEEE802MAC(t *testing.T) {
	t.Run("should parse a valid MAC address", func(t *testing.T) {
		mac := "00:00:5e:00:53:01"
		expected := [6]byte{0x00, 0x00, 0x5e, 0x00, 0x53, 0x01}
		actual, err := util.ParseIEEE802MAC(mac)
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)
	})

	t.Run("should return an error for an invalid MAC address", func(t *testing.T) {
		mac := "not a mac"
		_, err := util.ParseIEEE802MAC(mac)
		assert.Error(t, err)
	})
}

func TestValidateUint16Port(t *testing.T) {
	t.Run("should not return an error for a valid port", func(t *testing.T) {
		err := util.ValidateUint16Port(8080)
		assert.NoError(t, err)
	})

	t.Run("should return an error for an invalid port", func(t *testing.T) {
		err := util.ValidateUint16Port(0)
		assert.Error(t, err)

		err = util.ValidateUint16Port(65536)
		assert.Error(t, err)
	})
}
