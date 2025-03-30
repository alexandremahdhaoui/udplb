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
package bpfadapter

import "context"

func (lb *udplb) Run(ctx context.Context) error { panic("unimplemented") }

// func (lb *udplb) Run(ctx context.Context) error {
// 	// Remove resource limits for kernels <5.11.
// 	if err := rlimit.RemoveMemlock(); err != nil {
// 		log.Fatal("Removing memlock:", err)
// 	}
//
// 	// -- Init constants
//
// 	spec, err := loadUdplb()
// 	if err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
//
// 	if err = InitConstants(spec, cfg); err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
//
// 	// -- Load ebpf elf into kernel
// 	var objs udplbObjects
// 	if err = spec.LoadAndAssign(&objs, nil); err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
// 	defer objs.Close()
//
// 	if err = InitObjects(&objs, cfg); err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
//
// 	// -- Get iface
// 	iface, err := net.InterfaceByName(cfg.Ifname)
// 	if err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
//
// 	// -- Attach udplb to iface
// 	link, err := link.AttachXDP(link.XDPOptions{
// 		Program:   objs.Udplb,
// 		Interface: iface.Index,
// 	})
// 	if err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
// 	defer link.Close()
//
// 	slog.Info("XDP program loaded successfully", "ifname", cfg.Ifname)
//
// 	// -- 4. Fetch packet counter every second & print content.
// 	stop := make(chan os.Signal, 1)
// 	signal.Notify(stop, os.Interrupt)
//
// 	<-stop
// 	log.Printf("Stopping...")
//
// 	return nil
// }
//
// // -------------------------------------------------------------------
// // -- BPF INITIALIZATION
// // -------------------------------------------------------------------
//
// func InitConstants(spec *ebpf.CollectionSpec, cfg InitialConfig) error {
// 	ip, err := util.ParseIPToUint32(cfg.spec.IP)
// 	if err != nil {
// 		return err
// 	}
//
// 	if err := spec.Variables["UDPLB_IP"].Set(ip); err != nil {
// 		return err
// 	}
//
// 	return spec.Variables["UDPLB_PORT"].Set(cfg.Port)
// }
//
// func InitObjects(objs *udplbObjects, cfg types.Config) error {
// 	var nBackends uint32
//
// 	for i, t := range cfg.Backends {
// 		nBackends += 1
//
// 		ip, err := util.ParseIPToUint32(t.IP)
// 		if err != nil {
// 			return err
// 		}
//
// 		mac, err := util.ParseIEEE802MAC(t.MAC)
// 		if err != nil {
// 			return err
// 		}
//
// 		// TODO: parse mac addr
// 		backend := udplbBackendSpec{
// 			Mac:     mac,
// 			Ip:      ip,
// 			Port:    uint16(t.Port),
// 			Enabled: t.Enabled,
// 		}
//
// 		// -- Set backend in maps
// 		if err := objs.Backends.Put(uint32(i), &backend); err != nil {
// 			return err
// 		}
// 	}
//
// 	// Set n_backends
// 	if err := objs.N_backends.Set(nBackends); err != nil {
// 		return err
// 	}
//
// 	return nil
// }
