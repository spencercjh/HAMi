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

package api

import "testing"

type fakeUsageInfo struct{}

func (fakeUsageInfo) DeviceMax() int                     { return 0 }
func (fakeUsageInfo) DeviceNum() int                     { return 0 }
func (fakeUsageInfo) DeviceMemoryContextSize(int) uint64 { return 0 }
func (fakeUsageInfo) DeviceMemoryModuleSize(int) uint64  { return 0 }
func (fakeUsageInfo) DeviceMemoryBufferSize(int) uint64  { return 0 }
func (fakeUsageInfo) DeviceMemoryOffset(int) uint64      { return 0 }
func (fakeUsageInfo) DeviceMemoryTotal(int) uint64       { return 0 }
func (fakeUsageInfo) DeviceSmUtil(int) uint64            { return 0 }
func (fakeUsageInfo) SetDeviceSmLimit(uint64)            {}
func (fakeUsageInfo) IsValidUUID(int) bool               { return false }
func (fakeUsageInfo) DeviceUUID(int) string              { return "" }
func (fakeUsageInfo) DeviceMemoryLimit(int) uint64       { return 0 }
func (fakeUsageInfo) SetDeviceMemoryLimit(uint64)        {}
func (fakeUsageInfo) LastKernelTime() int64              { return 0 }
func (fakeUsageInfo) GetPriority() int                   { return 0 }
func (fakeUsageInfo) GetRecentKernel() int32             { return 0 }
func (fakeUsageInfo) SetRecentKernel(int32)              {}
func (fakeUsageInfo) GetUtilizationSwitch() int32        { return 0 }
func (fakeUsageInfo) SetUtilizationSwitch(int32)         {}

type fakeFactory struct {
	name  string
	match func(*Header, int64) bool
}

func (f fakeFactory) Match(h *Header, size int64) bool { return f.match(h, size) }
func (f fakeFactory) Cast([]byte) UsageInfo            { return fakeUsageInfo{} }
func (f fakeFactory) Name() string                     { return f.name }

func resetFactoriesForTest(t *testing.T) {
	t.Helper()
	factoriesMu.Lock()
	orig := append([]CacheFactory(nil), factories...)
	factories = nil
	factoriesMu.Unlock()
	t.Cleanup(func() {
		factoriesMu.Lock()
		factories = orig
		factoriesMu.Unlock()
	})
}

func TestFindFactoryReturnsNilWhenRegistryEmpty(t *testing.T) {
	resetFactoriesForTest(t)

	if got := FindFactory(&Header{MajorVersion: 1, MinorVersion: 0}, 64); got != nil {
		t.Fatalf("expected nil factory, got %q", got.Name())
	}
}

func TestRegisterFactoryAndFindFactory(t *testing.T) {
	resetFactoriesForTest(t)

	want := fakeFactory{
		name: "custom",
		match: func(h *Header, size int64) bool {
			return h.MajorVersion == 7 && h.MinorVersion == 3 && size == 64
		},
	}

	RegisterFactory(want)

	got := FindFactory(&Header{MajorVersion: 7, MinorVersion: 3}, 64)
	if got == nil {
		t.Fatal("expected registered factory, got nil")
	}
	if got.Name() != want.Name() {
		t.Fatalf("expected factory %q, got %q", want.Name(), got.Name())
	}
}
