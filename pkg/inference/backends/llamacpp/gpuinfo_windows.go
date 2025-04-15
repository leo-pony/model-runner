package llamacpp

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

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
