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
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/google/uuid"
)

type (
	orchestrator struct {
		// Number of total sessions.
		nSessions int
		// Number of producers per session.
		nProducersPerSession int
		// Number of loadbalancers.
		nLoadbalancers int
		// Number of recorders
		// The loadbalancer/backend ratio may not be 1-to-1.
		nRecorders int
	}

	producer struct {
		producerId uuid.UUID
		sessionId  uuid.UUID

		// Number of iteration per second.
		// Please note that one iteration may produce more than 1 packet.
		frequency int

		// Number of packets sent per iteration.
		// This can be useful to test loadbalancer's accuracy under high load.
		packetPerIteration int
	}

	// recorder will be a process in a vm.
	// we need to find a way to communicate with it.
	// (via GRPC?)
	recorder struct {
		PID int
		// IP of the recorder service running in the VM.
		ServiceIP net.IP
		// Port of the recorder service running in the VM.
		ServicePort int
	}
)

/******************************************************************************
 * Run
 *
 *
 ******************************************************************************/

func Run(cfg Config) error {
	sk, err := OpenTapFd(cfg.Taps[0].Name)
	if err != nil {
		return err
	}

	// -- test iface: write to the tap
	frameBuf := make([]byte, 1500)
	copy(frameBuf, "yolo")
	if err := sk.Sendto(frameBuf); err != nil {
		return err
	}

	go func() {
		cnx, err := net.DialUDP("udp", nil, &net.UDPAddr{
			IP:   net.ParseIP("10.0.0.10"),
			Port: 12345,
		})
		if err != nil {
			slog.Error("opening socket to 10.0.0.10:12345", "err", err.Error())
		}
		defer cnx.Close()

		for {
			if _, err := cnx.Write([]byte("yolo")); err != nil {
				slog.Error("sending udp packet to 10.0.0.10:12345", "err", err.Error())
			}

			slog.Info("send udp packet", "data", "yolo", "dest", "10.0.0.10:12345")
			time.Sleep(5 * time.Second)
		}
	}()

	go func() {
		// TODO: use a context and pass it the ifname instead of overloading
		//       the slog.Error() arguments.
		ifname := "tap0"
		cmd, err := startUDPLB(ifname)
		if err != nil {
			slog.Error(err.Error(), "ifname", ifname)
			// TODO: terminate the test
			return
		}

		if err := cmd.Wait(); err != nil {
			slog.Error(err.Error(), "ifname", ifname)
			// TODO: terminate the test
			return
		}

		// When we are done with the test, we can gracefully shutdown the loadbalancer:
		// This will also come in handy because we can also choose to down one of the
		// loadbalancer and see how the system continue behaving.
		// cmd.Process.Kill()
	}()

	for {
		// -- test ifcae read from the tap
		buf := make([]byte, 1500)
		_, err := sk.Recv(buf)
		if err != nil {
			return err
		}

		slog.Info("received packet")
	}
}

func startUDPLB(ifname string) (*exec.Cmd, error) {
	errCtx := fmt.Errorf("starting udplb on if %q", ifname)

	os.Setenv("CGO_ENABLED", "0")
	cmd := exec.Command("./udplb.o", "-")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, flaterrors.Join(err, errCtx)
	}

	template, err := os.ReadFile("./test/udplb.yaml")
	if err != nil {
		return nil, flaterrors.Join(err, errCtx)
	}

	cfgFile := []byte(fmt.Sprintf(string(template), ifname))
	if _, err = stdin.Write(cfgFile); err != nil {
		return nil, flaterrors.Join(err, errCtx)
	}

	stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, flaterrors.Join(err, errCtx)
	}

	stdoutScanner := bufio.NewScanner(stdout)
	go func() {
		for stdoutScanner.Scan() {
			fmt.Println(stdoutScanner.Text())
		}
	}()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, flaterrors.Join(err, errCtx)
	}

	stderrScanner := bufio.NewScanner(stderr)
	go func() {
		for stderrScanner.Scan() {
			fmt.Println(stderrScanner.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		return nil, flaterrors.Join(err, errCtx)
	}

	slog.Info("successfully started udplb", "on ifname", ifname)

	return cmd, nil
}

type Socket struct {
	sockfd   int
	sockAddr syscall.Sockaddr
}

func (sk Socket) Recv(buf []byte) (int, error) {
	// https://stackoverflow.com/questions/55274269/how-to-read-write-from-existing-tap-interface-in-c
	n, _, err := syscall.Recvfrom(sk.sockfd, buf, 0)
	if err != nil {
		return 0, err
	}

	return n, nil
}

func (sk Socket) Sendto(frame []byte) error {
	// Please note "frame" muse be a valid eth frame otherwise the syscall returns
	// errno invalid argument.
	if err := syscall.Sendto(sk.sockfd, frame, 0, sk.sockAddr); err != nil {
		return flaterrors.Join(err, errors.New("cannot write to socket"))
	}

	return nil
}

// Ifreq represents the struct ifreq from <net/if.h>
// - https://www.kernel.org/doc/Documentation/networking/tuntap.txt
// - sizeof(ifreq) -> 0x28
//
// <net/if.h>
// #define IF_NAMESIZE	16
// struct ifreq
// {
// # define IFHWADDRLEN	6
// # define IFNAMSIZ	IF_NAMESIZE
//
//		    union
//		      {
//			char ifrn_name[IFNAMSIZ];	/* Interface name, e.g. "en0".  */
//		      } ifr_ifrn;
//
//		    union
//		      {
//			struct sockaddr ifru_addr;
//			struct sockaddr ifru_dstaddr;
//			struct sockaddr ifru_broadaddr;
//			struct sockaddr ifru_netmask;
//			struct sockaddr ifru_hwaddr;
//			short int ifru_flags;
//			int ifru_ivalue;
//			int ifru_mtu;
//			struct ifmap ifru_map;
//			char ifru_slave[IFNAMSIZ];	/* Just fits the size */
//			char ifru_newname[IFNAMSIZ];
//			__caddr_t ifru_data;
//		      } ifr_ifru;
//	};
type Ifreq struct { // sizeof(ifreq) -> 40
	IfrName  [16]byte // 16 bytes
	IfrFlags IfrFlag  // 2 bytes
	_        [22]byte // 40 - (16 + 2)
}

type IfrFlag uint16

const (
	// #define IFF_TAP		0x0002
	IFF_TAP IfrFlag = 0x0002
	// #define IFF_NO_PI	0x1000
	IFF_NO_PI IfrFlag = 0x1000
)

// Open a tap device's file descriptor
func OpenTapFd(name string) (*Socket, error) {
	fd, err := syscall.Open("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	ifr := new(Ifreq)
	copy(ifr.IfrName[:], name)
	ifr.IfrFlags = IFF_TAP | IFF_NO_PI

	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		syscall.TUNSETIFF,
		uintptr(unsafe.Pointer(ifr)),
	); errno != 0 {
		return nil, os.NewSyscallError("ioctl", errno)
	}

	// #define ETH_P_ALL	0x0003		/* Every packet (be careful!!!) */
	sockfd, err := syscall.Socket(
		syscall.AF_PACKET,
		syscall.SOCK_RAW,
		int(htons(syscall.ETH_P_ALL)),
	)
	if err != nil {
		return nil, err
	}

	netif, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}

	// From github.com/golang/go/src/syscall/syscall_linux_amd64.go
	sa := syscall.SockaddrLinklayer{
		Protocol: htons(syscall.ETH_P_ALL),
		Ifindex:  netif.Index,
		Pkttype:  syscall.PACKET_HOST,
		// Hatype:   0,
		// Halen:    0,
		// Addr:     [8]byte{},
	}

	if err = syscall.Bind(sockfd, &sa); err != nil {
		return nil, err
	}

	return &Socket{
		sockfd:   sockfd,
		sockAddr: &sa,
	}, nil
}

func htons(i uint16) uint16 {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, i)
	return binary.BigEndian.Uint16(b)
}
