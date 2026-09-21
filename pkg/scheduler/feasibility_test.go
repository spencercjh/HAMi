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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	extenderv1 "k8s.io/kube-scheduler/extender/v1"

	"github.com/Project-HAMi/HAMi/pkg/device"
	"github.com/Project-HAMi/HAMi/pkg/device/nvidia"
	"github.com/Project-HAMi/HAMi/pkg/scheduler/config"
	"github.com/Project-HAMi/HAMi/pkg/scheduler/feasibility"
)

func feasibilityPod(name string, mem int64) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name), Annotations: map[string]string{"hami.io/gpu-scheduler-policy": "binpack"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1"), "nvidia.com/gpumem": *resource.NewQuantity(mem, resource.DecimalSI)}}}}}}
}
func feasibilityFixture(t *testing.T) (*Scheduler, feasibility.Request) {
	t.Helper()
	require.NoError(t, config.InitDevicesWithConfig(&config.Config{NvidiaConfig: nvidia.NvidiaConfig{ResourceCountName: "nvidia.com/gpu", ResourceMemoryName: "nvidia.com/gpumem", ResourceMemoryPercentageName: "nvidia.com/gpumem-percentage", ResourceCoreName: "nvidia.com/gpucores", DefaultGPUNum: 1}}))
	node := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Annotations: map[string]string{nvidia.RegisterAnnos: `[{"id":"GPU-0","count":10,"devmem":24576,"devcore":100,"type":"NVIDIA-T4","health":true,"mode":"hami-core"}]`}}}
	return NewScheduler(), feasibility.Request{ExtenderArgs: extenderv1.ExtenderArgs{Pod: feasibilityPod("candidate", 12288), Nodes: &corev1.NodeList{Items: []corev1.Node{node}}}, Simulation: &feasibility.Input{Version: feasibility.Version, Resources: []string{"nvidia.com/gpu", "nvidia.com/gpumem"}, Nodes: map[string][]feasibility.Resident{"node": {}}}}
}
func TestFeasibilityCumulativeOccupancy(t *testing.T) {
	s, r := feasibilityFixture(t)
	resident := feasibilityPod("resident", 18432)
	// The real legacy Filter accepts despite the production cache's occupancy.
	s.podManager.AddPod(resident, "node", device.PodDevices{nvidia.NvidiaGPUDevice: {{{UUID: "GPU-0", Type: nvidia.NvidiaGPUDevice, Usedmem: 18432}}}})
	legacy, err := s.Filter(r.ExtenderArgs)
	require.NoError(t, err)
	require.Len(t, legacy.Nodes.Items, 1)
	r.Simulation.Nodes["node"] = []feasibility.Resident{{Pod: resident, Allocation: feasibility.Allocation{Mode: feasibility.Allocate}}}
	before, _ := json.Marshal(r)
	result, err := s.FilterFeasibility(r)
	require.NoError(t, err)
	require.Empty(t, result.Nodes.Items)
	r.Pod = feasibilityPod("second", 18432)
	result, err = s.FilterFeasibility(r)
	require.NoError(t, err)
	require.Empty(t, result.Nodes.Items)
	r.Pod = feasibilityPod("candidate", 12288)
	after, _ := json.Marshal(r)
	require.JSONEq(t, string(before), string(after))
	r.Simulation.Nodes["node"] = []feasibility.Resident{}
	result, err = s.FilterFeasibility(r)
	require.NoError(t, err)
	require.Len(t, result.Nodes.Items, 1)
	cached, ok := s.podManager.GetPod(resident)
	require.True(t, ok)
	require.Equal(t, int32(18432), cached.Devices[nvidia.NvidiaGPUDevice][0][0].Usedmem)
}

func preserveResident(p *corev1.Pod, node, encoded string) feasibility.Resident {
	p = p.DeepCopy()
	p.Spec.NodeName = node
	data, _ := json.Marshal(allocationRecord{Node: node, PodUID: p.UID, Devices: encoded})
	return feasibility.Resident{Pod: p, Allocation: feasibility.Allocation{Mode: feasibility.Preserve, Encoding: allocationEncoding, Data: data}}
}
func TestFeasibilityPreservesFragmentationAndUserConstraints(t *testing.T) {
	s, r := feasibilityFixture(t)
	r.Nodes.Items[0].Annotations[nvidia.RegisterAnnos] = `[{"id":"GPU-0","count":10,"devmem":24576,"devcore":100,"type":"NVIDIA-T4","health":true,"mode":"hami-core"},{"id":"GPU-1","count":10,"devmem":24576,"devcore":100,"type":"NVIDIA-T4","health":true,"mode":"hami-core"}]`
	first, second := feasibilityPod("a", 12288), feasibilityPod("b", 12288)
	r.Pod = feasibilityPod("candidate", 18432)
	r.Simulation.Nodes["node"] = []feasibility.Resident{preserveResident(first, "node", "GPU-0,NVIDIA,12288,0:;"), preserveResident(second, "node", "GPU-1,NVIDIA,12288,0:;")}
	res, err := s.FilterFeasibility(r)
	require.NoError(t, err)
	require.Empty(t, res.Nodes.Items)
	for i := range r.Simulation.Nodes["node"] {
		r.Simulation.Nodes["node"][i].Allocation = feasibility.Allocation{Mode: feasibility.Allocate}
	}
	res, err = s.FilterFeasibility(r)
	require.NoError(t, err)
	require.Len(t, res.Nodes.Items, 1)
	r.Simulation.Nodes["node"][0].Pod.Annotations = map[string]string{nvidia.GPUUseUUID: "GPU-0"}
	r.Simulation.Nodes["node"][1].Pod.Annotations = map[string]string{nvidia.GPUUseUUID: "GPU-1", "hami.io/vgpu-devices-allocated": "stale"}
	res, err = s.FilterFeasibility(r)
	require.NoError(t, err)
	require.Empty(t, res.Nodes.Items)
}
func TestFeasibilityRejectsInvalidState(t *testing.T) {
	for _, name := range []string{"missing", "version", "record", "device", "node", "demand", "constraint", "resource"} {
		t.Run(name, func(t *testing.T) {
			s, r := feasibilityFixture(t)
			r.Simulation.Nodes["node"] = []feasibility.Resident{preserveResident(feasibilityPod("resident", 18432), "node", "GPU-0,NVIDIA,18432,0:;")}
			switch name {
			case "resource":
				r.Simulation.Resources = []string{"unknown.example/device"}
			case "missing":
				r.Simulation.Nodes["node"] = nil
			case "version":
				r.Simulation.Version = "unknown"
			case "record":
				r.Simulation.Nodes["node"][0].Allocation.Data = nil
			case "device":
				r.Simulation.Nodes["node"][0] = preserveResident(feasibilityPod("resident", 18432), "node", "GPU-missing,NVIDIA,18432,0:;")
			case "node":
				r.Simulation.Nodes["node"][0].Pod.Spec.NodeName = "other"
			case "demand":
				r.Simulation.Nodes["node"][0] = preserveResident(feasibilityPod("resident", 18432), "node", "GPU-0,NVIDIA,1,0:;")
			case "constraint":
				r.Simulation.Nodes["node"][0].Pod.Annotations = map[string]string{nvidia.GPUUseUUID: "other"}
			case "mig":
				r.Nodes.Items[0].Annotations[nvidia.RegisterAnnos] = `[{"id":"GPU-0","count":10,"devmem":24576,"devcore":100,"type":"NVIDIA-T4","health":true,"mode":"mig"}]`
			}
			_, err := s.FilterFeasibility(r)
			require.Error(t, err)
		})
	}
}
func TestFeasibilitySlotCoreAndInitSidecar(t *testing.T) {
	for _, mode := range []string{"slots", "cores", "init", "sidecar"} {
		t.Run(mode, func(t *testing.T) {
			s, r := feasibilityFixture(t)
			resident := feasibilityPod("resident", 1024)
			want := 0
			switch mode {
			case "slots":
				r.Nodes.Items[0].Annotations[nvidia.RegisterAnnos] = `[{"id":"GPU-0","count":1,"devmem":24576,"devcore":100,"type":"NVIDIA-T4","health":true,"mode":"hami-core"}]`
			case "cores":
				resident.Spec.Containers[0].Resources.Limits["nvidia.com/gpucores"] = resource.MustParse("80")
				r.Pod.Spec.Containers[0].Resources.Limits["nvidia.com/gpucores"] = resource.MustParse("30")
			case "init", "sidecar":
				resident = feasibilityPod("resident", 8192)
				init := feasibilityPod("init", 12288).Spec.Containers[0]
				init.Name = "init"
				resident.Spec.InitContainers = []corev1.Container{init}
				if mode == "sidecar" {
					always := corev1.ContainerRestartPolicyAlways
					resident.Spec.InitContainers[0].RestartPolicy = &always
				} else {
					want = 1
				}
			}
			r.Simulation.Nodes["node"] = []feasibility.Resident{{Pod: resident, Allocation: feasibility.Allocation{Mode: feasibility.Allocate}}}
			res, err := s.FilterFeasibility(r)
			require.NoError(t, err)
			require.Len(t, res.Nodes.Items, want)
		})
	}
}

func TestFeasibilityPreservedConcurrencyConstraints(t *testing.T) {
	for _, mode := range []string{"exclusive", "mutex", "mig metadata", "init", "sidecar"} {
		t.Run(mode, func(t *testing.T) {
			s, r := feasibilityFixture(t)
			a := feasibilityPod("a", 1024)
			wantError := true
			switch mode {
			case "exclusive":
				a.Spec.Containers[0].Resources.Limits["nvidia.com/gpucores"] = resource.MustParse("100")
				r.Simulation.Nodes["node"] = []feasibility.Resident{preserveResident(a, "node", "GPU-0,NVIDIA,1024,100:;"), preserveResident(feasibilityPod("b", 1024), "node", "GPU-0,NVIDIA,1024,0:;")}
			case "mutex":
				a.Annotations["hami.io/gpu-scheduler-policy"] = "mutex"
				a.Spec.Containers = append(a.Spec.Containers, a.Spec.Containers[0])
				a.Spec.Containers[1].Name = "second"
				r.Simulation.Nodes["node"] = []feasibility.Resident{preserveResident(a, "node", "GPU-0,NVIDIA,1024,0:;GPU-0,NVIDIA,1024,0:;")}
			case "mig metadata":
				a.Annotations[nvidia.MigAllocationsAnnotation] = "{}"
				r.Simulation.Nodes["node"] = []feasibility.Resident{preserveResident(a, "node", "GPU-0,NVIDIA,1024,0:;")}
			case "init", "sidecar":
				a = feasibilityPod("a", 8192)
				init := feasibilityPod("init", 12288).Spec.Containers[0]
				init.Name = "init"
				a.Spec.InitContainers = []corev1.Container{init}
				if mode == "sidecar" {
					always := corev1.ContainerRestartPolicyAlways
					a.Spec.InitContainers[0].RestartPolicy = &always
				}
				r.Simulation.Nodes["node"] = []feasibility.Resident{preserveResident(a, "node", "GPU-0,NVIDIA,12288,0:;GPU-0,NVIDIA,8192,0:;")}
				wantError = false
			}
			res, err := s.FilterFeasibility(r)
			if wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if mode == "init" {
					require.Len(t, res.Nodes.Items, 1)
				} else {
					require.Empty(t, res.Nodes.Items)
				}
			}
		})
	}
}

func TestFeasibilityMultipleCandidatesIgnoreUnusableInventory(t *testing.T) {
	s, r := feasibilityFixture(t)
	missing := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "missing"}}
	mig := *r.Nodes.Items[0].DeepCopy()
	mig.Name = "mig"
	mig.Annotations[nvidia.RegisterAnnos] = `[{"id":"GPU-MIG","count":10,"devmem":24576,"devcore":100,"type":"NVIDIA-A100","health":true,"mode":"mig"}]`
	r.Nodes.Items = append(r.Nodes.Items, missing, mig)
	r.Simulation.Nodes["missing"] = []feasibility.Resident{}
	r.Simulation.Nodes["mig"] = []feasibility.Resident{}
	res, err := s.FilterFeasibility(r)
	require.NoError(t, err)
	require.Len(t, res.Nodes.Items, 1)
	require.Equal(t, "node", res.Nodes.Items[0].Name)
	require.Len(t, res.FailedNodes, 2)
}

func TestFeasibilityRejectsUnrepresentableDemand(t *testing.T) {
	for _, resident := range []bool{false, true} {
		for name, value := range map[string]string{"nvidia.com/gpucores": "101", "nvidia.com/gpumem": "2147483648", "nvidia.com/gpu": "2147483648", "nvidia.com/gpumem-percentage": "-1"} {
			t.Run(fmt.Sprintf("resident=%t/%s", resident, name), func(t *testing.T) {
				s, r := feasibilityFixture(t)
				invalid := feasibilityPod("invalid", 1024)
				invalid.Spec.Containers[0].Resources.Limits[corev1.ResourceName(name)] = resource.MustParse(value)
				if resident {
					r.Simulation.Nodes["node"] = []feasibility.Resident{{Pod: invalid, Allocation: feasibility.Allocation{Mode: feasibility.Allocate}}}
				} else {
					r.Pod = invalid
				}
				_, err := s.FilterFeasibility(r)
				require.Error(t, err)
			})
		}
	}
}
