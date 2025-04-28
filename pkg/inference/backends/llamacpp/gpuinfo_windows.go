package llamacpp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/jaypipes/ghw"
)

func hasNVIDIAGPU() (bool, error) {
	gpus, err := ghw.GPU()
	if err != nil {
		return false, err
	}
	for _, gpu := range gpus.GraphicsCards {
		if strings.ToLower(gpu.DeviceInfo.Vendor.Name) == "nvidia" {
			return true, nil
		}
	}
	return false, nil
}

func hasCUDA11CapableGPU(ctx context.Context, nvGPUInfoBin string) (bool, error) {
	nvGPU, err := hasNVIDIAGPU()
	if !nvGPU || err != nil {
		return false, err
	}
	cmd := exec.CommandContext(ctx, nvGPUInfoBin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, err
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		version, found := strings.CutPrefix(sc.Text(), "driver version:")
		if found {
			version = strings.TrimSpace(version)
			if len(version) != 5 {
				return false, fmt.Errorf("unexpected NVIDIA driver version format: %s", version)
			}
			major, err := strconv.Atoi(version[:3])
			if err != nil {
				return false, fmt.Errorf("unexpected NVIDIA driver version format: %s", version)
			}
			minor, err := strconv.Atoi(version[3:5])
			if err != nil {
				return false, fmt.Errorf("unexpected NVIDIA driver version format: %s", version)
			}
			return major > 452 || (major == 452 && minor >= 39), nil
		}
	}
	return false, nil
}

func hasOpenCL() (bool, error) {
	opencl, err := syscall.LoadLibrary("OpenCL.dll")
	if err != nil {
		if errors.Is(err, syscall.ERROR_MOD_NOT_FOUND) {
			return false, nil
		}
		return false, fmt.Errorf("unable to load OpenCL DLL: %w", err)
	}
	// We could perform additional platform and device version checks here (if
	// we scaffold out the relevant OpenCL API datatypes in Go), but since users
	// can opt-out of GPU support, we can probably skip that and just let users
	// disable it if things don't work. Alternatively, we could inspect the GPUs
	// found by the ghw package, if it supports (e.g.) Adreno GPUs.
	syscall.FreeLibrary(opencl)
	return true, nil
}

func CanUseGPU(ctx context.Context, nvGPUInfoBin string) (bool, error) {
	haveCUDA11GPU, err := hasCUDA11CapableGPU(ctx, nvGPUInfoBin)
	if haveCUDA11GPU || err != nil {
		return haveCUDA11GPU, err
	}
	return hasOpenCL()
}
