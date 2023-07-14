package utils

import (
	"fmt"

	"github.com/cilium/ebpf/features"
)

type KernelVersion uint32

func (v KernelVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch())
}

func (v KernelVersion) Major() uint8 {
	return (uint8)(v >> 16)
}

func (v KernelVersion) Minor() uint8 {
	return (uint8)((v >> 8) & 0xff)
}

func (v KernelVersion) Patch() uint8 {
	return (uint8)(v & 0xff)
}

func HostKernelVersion() (KernelVersion, error) {
	code, err := features.LinuxVersionCode()
	if err != nil {
		return 0, err
	}
	return KernelVersion(code), nil
}
