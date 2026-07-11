/*
Copyright 2024 The HAMi Authors.

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

package nvidia

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/Project-HAMi/HAMi/pkg/monitor/nvidia/api"
)

type testUsageInfo struct {
	priority int
}

func (u *testUsageInfo) DeviceMax() int                     { return 0 }
func (u *testUsageInfo) DeviceNum() int                     { return 0 }
func (u *testUsageInfo) DeviceMemoryContextSize(int) uint64 { return 0 }
func (u *testUsageInfo) DeviceMemoryModuleSize(int) uint64  { return 0 }
func (u *testUsageInfo) DeviceMemoryBufferSize(int) uint64  { return 0 }
func (u *testUsageInfo) DeviceMemoryOffset(int) uint64      { return 0 }
func (u *testUsageInfo) DeviceMemoryTotal(int) uint64       { return 0 }
func (u *testUsageInfo) DeviceSmUtil(int) uint64            { return 0 }
func (u *testUsageInfo) SetDeviceSmLimit(uint64)            {}
func (u *testUsageInfo) IsValidUUID(int) bool               { return false }
func (u *testUsageInfo) DeviceUUID(int) string              { return "" }
func (u *testUsageInfo) DeviceMemoryLimit(int) uint64       { return 0 }
func (u *testUsageInfo) SetDeviceMemoryLimit(uint64)        {}
func (u *testUsageInfo) LastKernelTime() int64              { return 0 }
func (u *testUsageInfo) GetPriority() int                   { return u.priority }
func (u *testUsageInfo) GetRecentKernel() int32             { return 0 }
func (u *testUsageInfo) SetRecentKernel(int32)              {}
func (u *testUsageInfo) GetUtilizationSwitch() int32        { return 0 }
func (u *testUsageInfo) SetUtilizationSwitch(int32)         {}

type loadCacheFactory struct{}

func (loadCacheFactory) Match(h *api.Header, size int64) bool {
	return h.MajorVersion == 77 && h.MinorVersion == 3 && size == 64
}

func (loadCacheFactory) Cast([]byte) api.UsageInfo { return &testUsageInfo{priority: 42} }
func (loadCacheFactory) Name() string              { return "load-cache-test" }

func TestLoadCacheRoutesThroughRegisteredFactory(t *testing.T) {
	t.Helper()

	RegisterFactory(loadCacheFactory{})

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "custom.cache")
	data := make([]byte, 64)
	header := (*HeaderT)(unsafe.Pointer(&data[0]))
	header.InitializedFlag = SharedRegionMagicFlag
	header.MajorVersion = 77
	header.MinorVersion = 3

	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	usage, err := loadCache(dir)
	if err != nil {
		t.Fatalf("loadCache returned error: %v", err)
	}
	if usage == nil {
		t.Fatal("expected usage, got nil")
	}
	got, ok := usage.Info.(*testUsageInfo)
	if !ok {
		t.Fatalf("expected testUsageInfo, got %T", usage.Info)
	}
	if got.GetPriority() != 42 {
		t.Fatalf("expected priority 42, got %d", got.GetPriority())
	}
}

func TestNewContainerListerRequiresHookPath(t *testing.T) {
	oldHookPath, hadHookPath := os.LookupEnv("HOOK_PATH")
	oldKubeconfig, hadKubeconfig := os.LookupEnv("KUBECONFIG")
	oldNodeName, hadNodeName := os.LookupEnv("NODE_NAME")
	t.Cleanup(func() {
		if hadHookPath {
			_ = os.Setenv("HOOK_PATH", oldHookPath)
		} else {
			_ = os.Unsetenv("HOOK_PATH")
		}
		if hadKubeconfig {
			_ = os.Setenv("KUBECONFIG", oldKubeconfig)
		} else {
			_ = os.Unsetenv("KUBECONFIG")
		}
		if hadNodeName {
			_ = os.Setenv("NODE_NAME", oldNodeName)
		} else {
			_ = os.Unsetenv("NODE_NAME")
		}
	})

	_ = os.Unsetenv("HOOK_PATH")
	_ = os.Setenv("KUBECONFIG", "")
	_ = os.Setenv("NODE_NAME", "test-node")

	_, err := NewContainerLister()
	if err == nil || err.Error() != "HOOK_PATH not set" {
		t.Fatalf("expected HOOK_PATH error, got %v", err)
	}
}
