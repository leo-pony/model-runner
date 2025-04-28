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

// supportedAdrenoGPUVersions are the Adreno versions supported by llama.cpp.
// See (and keep in sync with):
// https://github.com/ggml-org/llama.cpp/blob/43ddab6eeeaab5a04fe5a364af0bafb0e4d35065/ggml/src/ggml-opencl/ggml-opencl.cpp#L180-L196
var supportedAdrenoGPUVersions = []string{
	"730",
	"740",
	"750",
	"830",
	"X1",
}

func hasSupportedAdrenoGPU() (bool, error) {
	gpus, err := ghw.GPU()
	if err != nil {
		return false, err
	}
	for _, gpu := range gpus.GraphicsCards {
		isAdrenoFamily := strings.Contains(gpu.DeviceInfo.Product.Name, "Adreno") ||
			strings.Contains(gpu.DeviceInfo.Product.Name, "Qualcomm")
		if !isAdrenoFamily {
			continue
		}
		for _, version := range supportedAdrenoGPUVersions {
			if strings.Contains(gpu.DeviceInfo.Product.Name, version) {
				return true, nil
			}
		}
	}
	return false, nil
}

func hasOpenCL() (bool, error) {
	// We compile our llama.cpp backend with Adreno-specific kernels, so for now
	// we don't support OpenCL on other GPUs.
	adrenoGPU, err := hasSupportedAdrenoGPU()
	if !adrenoGPU || err != nil {
		return false, err
	}

	// Check for an OpenCL implementation.
	opencl, err := syscall.LoadLibrary("OpenCL.dll")
	if err != nil {
		if errors.Is(err, syscall.ERROR_MOD_NOT_FOUND) {
			return false, nil
		}
		return false, fmt.Errorf("unable to load OpenCL DLL: %w", err)
	}
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
