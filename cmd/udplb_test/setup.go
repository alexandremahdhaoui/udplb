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

	// -- Create bridges
	bridges := cfg.Bridges()
	if len(bridges) == 0 {
		return errors.New("no bridges defined in config")
	}

	// Map bridge names to netlink.Bridge for TAP device setup
	bridgeMap := make(map[string]*netlink.Bridge)

	for _, brSpec := range bridges {
		br := &netlink.Bridge{LinkAttrs: netlink.NewLinkAttrs()}
		br.Name = brSpec.Name
		br.Flags |= net.FlagUp

		if err = netlink.LinkAdd(br); err != nil {
			return flaterrors.Join(err, fmt.Errorf("cannot add link %q", br.Name))
		}

		slog.Info("successfully created bridge", "ifname", br.Name)

		if brSpec.IP != "" {
			brAddr, err := netlink.ParseAddr(brSpec.IP)
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
		}

		bridgeMap[brSpec.Name] = br
	}

	// Use first bridge as the primary for ECMP route setup
	primaryBridge := bridgeMap[bridges[0].Name]

	// [ECMP] Set up anycast.
	ecmpRoute := &netlink.Route{
		Dst: &net.IPNet{
			IP:   net.ParseIP("10.0.0.123"), // TODO: LOADBALANCER IP!!!
			Mask: net.CIDRMask(32, 32),
		},
		MultiPath: nil,
	}

	// -- Create TAP devices
	for _, tap := range cfg.Taps() {
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

		// Set IP address if specified
		if tap.IP != "" {
			tapAddr, err := netlink.ParseAddr(tap.IP)
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

			// Add to ECMP route multipath
			ecmpRoute.MultiPath = append(ecmpRoute.MultiPath, &netlink.NexthopInfo{
				Via: &netlink.Via{
					AddrFamily: netlink.FAMILY_V4,
					Addr:       tapAddr.IP.To4(),
				},
			})
		}

		// Set master bridge (using Link field which maps to "link" or "master" in config)
		masterName := tap.Link
		if masterName == "" {
			// Default to primary bridge if no link specified
			masterName = primaryBridge.Name
		}
		if masterBr, ok := bridgeMap[masterName]; ok {
			if err := netlink.LinkSetMaster(tuntap, masterBr); err != nil {
				return flaterrors.Join(
					err,
					fmt.Errorf("cannot set link b/w %q & %q", tuntap.Name, masterBr.Attrs().Name),
				)
			}
			slog.Info("successfully set link", "subject", tuntap.Name, "leader", masterBr.Name)
		}
	}

	// [ECMP] Set up anycast advertised via BGP
	if len(ecmpRoute.MultiPath) > 0 {
		if err := netlink.RouteAdd(ecmpRoute); err != nil {
			return flaterrors.Join(err, errors.New("cannot add route"))
		}
		slog.Info("successfully set ECMP route")
	}

	return nil
}

/******************************************************************************
 * prepareVMs
 *
 *
 ******************************************************************************/

func prepareVMs(cfg Config) error {
	// VM preparation is not yet implemented.
	// The E2E test infrastructure requires QEMU VMs with Alpine Linux images,
	// multi-NIC networking (PublicNet + PrivateNet), and macvlan device creation.
	// This is tracked as future work separate from the forge migration.
	if len(cfg.VMs.Specs) > 0 {
		slog.Warn("VM preparation not implemented, skipping VM setup",
			"vmCount", len(cfg.VMs.Specs),
			"baseImage", cfg.VMs.BaseImage)
	}
	return nil
}
