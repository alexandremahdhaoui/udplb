package util

import (
	"encoding/binary"
	"errors"
	"net"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
)

// -------------------------------------------------------------------
// -- ParseIPToUint32
// -------------------------------------------------------------------

// TODO: add support for ipv6
func ParseIPToUint32(s string) (uint32, error) {
	ipv4 := net.ParseIP(s).To4()
	if ipv4 == nil {
		return 0, errors.New("IP must be ipv4")
	}

	return binary.NativeEndian.Uint32(ipv4), nil
}

var errParseIEEE802MAC = errors.New(
	"loadbalancer backend mac addr must be a valid IEEE 802 MAC address",
)

// -------------------------------------------------------------------
// -- ParseIEEE802MAC
// -------------------------------------------------------------------

func ParseIEEE802MAC(s string) ([6]uint8, error) {
	mac, err := net.ParseMAC(s)
	if err != nil {
		return [6]uint8{}, flaterrors.Join(err, errParseIEEE802MAC)
	}

	if len(mac) != 6 {
		return [6]uint8{}, errParseIEEE802MAC
	}

	return [6]uint8{
		mac[0],
		mac[1],
		mac[2],
		mac[3],
		mac[4],
		mac[5],
	}, nil
}
