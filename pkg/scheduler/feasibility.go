/*
Copyright 2026 The HAMi Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scheduler

import (
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	extenderv1 "k8s.io/kube-scheduler/extender/v1"

	"github.com/Project-HAMi/HAMi/pkg/device"
	"github.com/Project-HAMi/HAMi/pkg/device/nvidia"
	"github.com/Project-HAMi/HAMi/pkg/scheduler/feasibility"
	"github.com/Project-HAMi/HAMi/pkg/util"
)

// FilterFeasibility evaluates complete node-local occupancy without reading or
// writing scheduling caches. Alpha support is limited to NVIDIA hami-core;
// namespace quota and other policies needing nonlocal state are outside this input.
func (s *Scheduler) FilterFeasibility(r feasibility.Request) (*extenderv1.ExtenderFilterResult, error) {
	if err := feasibility.Validate(r); err != nil {
		return nil, err
	}
	backend, ok := device.GetDevices()[nvidia.NvidiaGPUDevice].(*nvidia.NvidiaGPUDevices)
	if !ok {
		return nil, fmt.Errorf("NVIDIA evaluator is not configured")
	}
	supported := make(map[string]bool)
	for _, name := range backend.FeasibilityResourceNames() {
		if name != "" {
			supported[name] = true
		}
	}
	for _, name := range r.Simulation.Resources {
		if !supported[name] {
			return nil, fmt.Errorf("unsupported managed resource %q", name)
		}
	}
	result := &extenderv1.ExtenderFilterResult{Nodes: &corev1.NodeList{}, FailedNodes: extenderv1.FailedNodesMap{}}
nodes:
	for i := range r.Nodes.Items {
		node := &r.Nodes.Items[i]
		info, err := buildTransientNodeInfo(node)
		if err != nil {
			result.FailedNodes[node.Name] = err.Error()
			continue
		}
		info.NodeLocalEvaluation = true
		usage := buildNodeUsage(info, r.Pod)
		for _, d := range usage.Devices.DeviceLists {
			if !deviceTypeMatches(d.Device.Type, nvidia.NvidiaGPUDevice) || d.Device.Mode != "hami-core" {
				result.FailedNodes[node.Name] = "feasibility supports only NVIDIA hami-core inventory"
				continue nodes
			}
		}
		residents := append([]feasibility.Resident(nil), r.Simulation.Nodes[node.Name]...)
		sort.Slice(residents, func(i, j int) bool { return residents[i].Pod.UID < residents[j].Pod.UID })
		for _, resident := range residents {
			if err := validateFeasibilityResources(resident.Pod, backend); err != nil {
				return nil, err
			}
			if resident.Allocation.Mode == feasibility.Preserve {
				if err := s.preserveFeasibility(usage, resident); err != nil {
					return nil, fmt.Errorf("preserved allocation: %w", err)
				}
			}
		}
		pending := make([]*corev1.Pod, 0, len(residents)+1)
		for _, resident := range residents {
			if resident.Allocation.Mode == feasibility.Allocate {
				pending = append(pending, resident.Pod)
			}
		}
		pending = append(pending, r.Pod)
		fits := true
		for _, p := range pending {
			if err := validateFeasibilityResources(p, backend); err != nil {
				return nil, err
			}
			task := freshFeasibilityPod(p)
			usage.Devices.Policy = util.GetGPUSchedulerPolicyByPod(device.GPUSchedulerPolicy, task)
			usage.Devices.NumaBind = numaBindingRequested(task)
			weights, err := util.GetDeviceScoringWeightsByPod(task)
			if err != nil {
				return nil, err
			}
			scored := s.scoreNode(node.Name, usage, device.Resourcereqs(task), task, resolveNodeSchedulerPolicy(task), weights)
			if scored.err != nil {
				return nil, scored.err
			}
			if scored.score == nil {
				result.FailedNodes[node.Name] = scored.reason
				fits = false
				break
			}
			// scoreNode updates the local peak occupancy using the same init/sidecar
			// rules as scheduling. Its result must never be added to the live PodManager.
		}
		if fits {
			result.Nodes.Items = append(result.Nodes.Items, *node.DeepCopy())
		}
	}
	return result, nil
}
func validateFeasibilityResources(p *corev1.Pod, backend *nvidia.NvidiaGPUDevices) error {
	for _, containers := range [][]corev1.Container{p.Spec.InitContainers, p.Spec.Containers} {
		for i := range containers {
			if err := backend.ValidateFeasibilityRequest(&containers[i]); err != nil {
				return err
			}
		}
	}
	for _, reqs := range device.Resourcereqs(p) {
		for kind := range reqs {
			if kind != nvidia.NvidiaGPUDevice {
				return fmt.Errorf("unsupported feasibility device type %s", kind)
			}
		}
	}
	return nil
}
func freshFeasibilityPod(p *corev1.Pod) *corev1.Pod {
	copy := p.DeepCopy()
	copy.Spec.NodeName = ""
	for _, key := range []string{util.AssignedNodeAnnotations, util.AssignedTimeAnnotations, device.InRequestDevices[nvidia.NvidiaGPUDevice], device.SupportDevices[nvidia.NvidiaGPUDevice], nvidia.MigAllocationsAnnotation} {
		delete(copy.Annotations, key)
	}
	return copy
}
func (s *Scheduler) preserveFeasibility(usage *NodeUsage, resident feasibility.Resident) error {
	if resident.Pod.Spec.NodeName != usage.Node.Name {
		return fmt.Errorf("preserved Pod binding does not match target node")
	}
	reqs := device.Resourcereqs(resident.Pod)
	key := device.SupportDevices[nvidia.NvidiaGPUDevice]
	record := resident.Pod.Annotations[key]
	hasRequest := false
	for _, requests := range reqs {
		hasRequest = hasRequest || requests[nvidia.NvidiaGPUDevice].Nums > 0
	}
	// Unrelated residents carry full context but need no NVIDIA assignment.
	// Stale allocation metadata on such a Pod is still checked, not discarded.
	if !hasRequest && record == "" && resident.Pod.Annotations[nvidia.MigAllocationsAnnotation] == "" {
		return nil
	}
	if record == "" || resident.Pod.Annotations[util.AssignedNodeAnnotations] != usage.Node.Name {
		return fmt.Errorf("missing or inconsistent preserved NVIDIA allocation metadata")
	}
	raw, err := device.DecodePodDevices(map[string]string{nvidia.NvidiaGPUDevice: key}, resident.Pod.Annotations)
	if err != nil {
		return err
	}
	rows := raw[nvidia.NvidiaGPUDevice]
	// The text encoding has one trailing semicolon, which decodes as an empty row.
	if len(rows) == len(reqs)+1 && len(rows[len(rows)-1]) == 0 {
		rows = rows[:len(rows)-1]
		raw[nvidia.NvidiaGPUDevice] = rows
	}
	if len(rows) != len(reqs) {
		return fmt.Errorf("allocation container count mismatch")
	}
	task := freshFeasibilityPod(resident.Pod)
	plugin := device.GetDevices()[nvidia.NvidiaGPUDevice]
	working := usage.DeepCopy()
	peak := snapshotPeakUsage(usage.Devices)
	allocated := device.PodDevices{nvidia.NvidiaGPUDevice: {}}
	for i, row := range rows {
		req := reqs[i][nvidia.NvidiaGPUDevice]
		if int(req.Nums) != len(row) {
			return fmt.Errorf("allocation device count mismatch")
		}
		phase := working
		if i < len(task.Spec.InitContainers) && !util.IsSidecarContainer(&task.Spec.InitContainers[i]) {
			phase = working.DeepCopy()
		}
		if len(row) == 0 {
			allocated[nvidia.NvidiaGPUDevice] = append(allocated[nvidia.NvidiaGPUDevice], nil)
			continue
		}
		allowed := make(map[string]device.ContainerDevice, len(row))
		for _, allocation := range row {
			if allocation.Type != nvidia.NvidiaGPUDevice || allocation.Slots > 1 || allocation.Usedmem < 0 || allocation.Usedcores < 0 {
				return fmt.Errorf("invalid raw device allocation")
			}
			if _, ok := allowed[allocation.UUID]; ok {
				return fmt.Errorf("duplicate device allocation")
			}
			allowed[allocation.UUID] = allocation
		}
		devices := make([]*device.DeviceUsage, 0, len(row))
		for _, d := range phase.Devices.DeviceLists {
			if _, ok := allowed[d.Device.ID]; ok {
				devices = append(devices, d.Device)
			}
		}
		if len(devices) != len(row) {
			return fmt.Errorf("unknown allocated device")
		}
		// Fit validates vendor, UUID, NUMA, memory-percentage and core constraints
		// against only the recorded devices, never an alternative packing.
		fit, chosen, _ := plugin.Fit(devices, req, task, usage.NodeInfo, &allocated)
		if !fit || len(chosen[nvidia.NvidiaGPUDevice]) != len(row) {
			return fmt.Errorf("record violates device constraints")
		}
		for _, a := range chosen[nvidia.NvidiaGPUDevice] {
			want, ok := allowed[a.UUID]
			if !ok || want.Usedmem != a.Usedmem || want.Usedcores != a.Usedcores {
				return fmt.Errorf("record does not match resource request")
			}
		}
		for j := range row {
			for _, d := range devices {
				if d.ID == row[j].UUID {
					if err := plugin.AddResourceUsage(task, d, &row[j]); err != nil {
						return err
					}
				}
			}
		}
		allocated[nvidia.NvidiaGPUDevice] = append(allocated[nvidia.NvidiaGPUDevice], row)
		updatePeakUsage(peak, phase)
	}
	if resident.Pod.Annotations[nvidia.MigAllocationsAnnotation] != "" {
		return fmt.Errorf("MIG allocation metadata is unsupported")
	}
	applyPeakUsage(usage, working, peak)
	return nil
}
