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

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/vishvananda/netlink"
)

/******************************************************************************
 * Setup
 *
 *
 ******************************************************************************/

// TODO: set interface state up
// TODO: set loopback interface state up
// TODO: add routes
func Setup(cfg Config) error {
	if err := setupNetwork(cfg); err != nil {
		return nil
	}

	return prepareVMs(cfg)
}

/******************************************************************************
 * setupNetwork
 *
 *
 ******************************************************************************/

func setupNetwork(cfg Config) error {
	// Loopback

	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return errors.New("cannot get loopback interface by name")
	}

	if err = netlink.LinkSetUp(lo); err != nil {
		return flaterrors.Join(err, fmt.Errorf("cannot set link %q up", lo.Attrs().Name))
	}

	// -- Bridge

	br := &netlink.Bridge{LinkAttrs: netlink.NewLinkAttrs()}
	br.Name = cfg.Bridge.Name
	br.Flags |= net.FlagUp

	if err = netlink.LinkAdd(br); err != nil {
		return flaterrors.Join(err, fmt.Errorf("cannot add linkr %q", br.Name))
	}

	slog.Info("successfully created bridge", "ifname", br.Name)

	brAddr, err := netlink.ParseAddr(cfg.Bridge.Addr)
	if err != nil {
		return flaterrors.Join(err, fmt.Errorf("cannot parse cidr for %q", br.Name))
	}

	if err = netlink.AddrAdd(br, brAddr); err != nil {
		return flaterrors.Join(err, fmt.Errorf("cannot set ip to %q", br.Name))
	}

	mask, _ := brAddr.Mask.Size()
	slog.Info(
		"successfully set ip addr",
		"ifname", br.Name,
		"ip", fmt.Sprintf("%s/%d", brAddr.IP.String(), mask),
	)

	// ip r add 10.0.0.0/24 via 10.0.0.1 // br0
	//
	// subnetRoute := &netlink.Route{
	// 	LinkIndex: br.Index,
	// 	Dst:       ipnet,
	// 	Via: &netlink.Via{
	// 		AddrFamily: netlink.FAMILY_V4,
	// 		Addr:       brAddr.IP,
	// 	},
	// }
	// if err = netlink.RouteAdd(subnetRoute); err != nil {
	// 	return flaterrors.Join(err, errors.New("cannot add route"))
	// }
	// slog.Info("successfully set route", "dest", brAddr.IPNet.String(), "via", brAddr.IP)

	// [ECMP] Set up anycast.
	ecmpRoute := &netlink.Route{
		// LinkIndex: br.Index,
		Dst: &net.IPNet{
			IP:   net.ParseIP("10.0.0.123"), // TODO: LOADBALANCER IP!!!
			Mask: net.CIDRMask(32, 32),
		},
		MultiPath: nil,
	}

	for _, tap := range cfg.Taps {
		tuntap := &netlink.Tuntap{
			LinkAttrs: netlink.NewLinkAttrs(),
			Mode:      netlink.TUNTAP_MODE_TAP,
		}

		tuntap.Name = tap.Name

		if err = netlink.LinkAdd(tuntap); err != nil {
			return flaterrors.Join(err, fmt.Errorf("cannot create tap device %q", tuntap.Name))
		}

		slog.Info("successfully created tap device", "ifname", tuntap.Name)

		if err = netlink.LinkSetUp(tuntap); err != nil {
			return flaterrors.Join(err, fmt.Errorf("cannot set link %q up", tuntap.Name))
		}

		tapAddr, err := netlink.ParseAddr(tap.Addr)
		if err != nil {
			return flaterrors.Join(err, fmt.Errorf("cannot parse cidr for %q", tap.Name))
		}

		if err := netlink.AddrAdd(tuntap, tapAddr); err != nil {
			return flaterrors.Join(err, fmt.Errorf("cannot set ip to %q", tap.Name))
		}

		mask, _ := tapAddr.Mask.Size()
		slog.Info(
			"successfully set ip addr",
			"ifname", tap.Name,
			"ip", fmt.Sprintf("%s/%d", tapAddr.IP.String(), mask),
		)

		if err := netlink.LinkSetMaster(tuntap, br); err != nil {
			return flaterrors.Join(
				err,
				fmt.Errorf("cannot set link b/w %q & %q", tuntap.Name, br.Attrs().Name),
			)
		}

		slog.Info("successfully set link", "subject", tuntap.Name, "leader", br.Name)

		// [ECMP] Set up anycast.
		// "hard-code" the routes:
		// - ip route add ${LB_IP}/32 nexthop via ${TAP_IP_0} dev br0 weigth 1
		// - ip route append ${LB_IP}/32 nexthop via ${TAP_IP_1} dev br0 weigth 1
		// - ip route append ${LB_IP}/32 nexthop via ${TAP_IP_2} dev br0 weigth 1
		// This should emulate the ECMP setup via anycast. This one is hard-coded
		// instead of advertised by BGP
		ecmpRoute.MultiPath = append(ecmpRoute.MultiPath, &netlink.NexthopInfo{
			// LinkIndex: br.Index,
			Via: &netlink.Via{
				AddrFamily: netlink.FAMILY_V4,
				Addr:       tapAddr.IP.To4(), // strangely IP must be set with To4() otherwise will fail :shrug:
			},
		})

	}

	// [ECMP] Set up anycast advertised via BGP
	// With a VM attached to the tap we can easily run a listener & a bgp daemon.
	//  - https://github.com/osrg/gobgp/tree/master || https://github.com/osrg/rustybgp
	//	- https://www.linuxjournal.com/magazine/ipv4-anycast-linux-and-quagga
	// # 1. Configure loopback interface with anycast IP
	// ip addr add ${LB_IP}/32 dev lo
	//
	// # 2. Configure BGP neighbor (replace details with your network)
	// router bgp 65000
	//  neighbor 10.0.0.1 remote-as 65001
	//
	// # 3. Advertise the anycast route
	// network ${LB_IP}/32
	if err := netlink.RouteAdd(ecmpRoute); err != nil {
		return flaterrors.Join(err, errors.New("cannot add route"))
	}

	slog.Info("successfully set ECMP route")

	return nil
}

/******************************************************************************
 * prepareVMs
 *
 *
 ******************************************************************************/

func prepareVMs(cfg Config) error {
	panic("unimplemented")
}
