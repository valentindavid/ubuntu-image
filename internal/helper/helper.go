package helper

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/snapcore/snapd/osutil"
)

// CaptureStd returns an io.Reader to read what was printed, and teardown
func CaptureStd(toCap **os.File) (io.Reader, func(), error) {
	stdCap, stdCapW, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	oldStdCap := *toCap
	*toCap = stdCapW
	closed := false
	return stdCap, func() {
		// only teardown once
		if closed {
			return
		}
		*toCap = oldStdCap
		stdCapW.Close()
		closed = true
	}, nil
}

// InitCommonOpts initializes default common options for state machines.
// This is used for test scenarios to avoid nil pointer dereferences
func InitCommonOpts() (*commands.CommonOpts, *commands.StateMachineOpts) {
	return new(commands.CommonOpts), new(commands.StateMachineOpts)
}

// GetHostArch uses dpkg to return the host architecture of the current system
func GetHostArch() string {
	cmd := exec.Command("dpkg", "--print-architecture")
	outputBytes, _ := cmd.Output()
	return strings.TrimSpace(string(outputBytes))
}

// getQemuStaticForArch returns the name of the qemu binary for the specified arch
func getQemuStaticForArch(arch string) string {
	archs := map[string]string{
		"armhf":   "qemu-arm-static",
		"arm64":   "qemu-aarch64-static",
		"ppc64el": "qemu-ppc64le-static",
	}
	if static, exists := archs[arch]; exists {
		return static
	}
	return ""
}

// RunLiveBuild creates and executes the live build commands used in classic images
func RunLiveBuild(rootfs, arch string, env []string, enableCrossBuild bool) error {
	autoSrc := os.Getenv("UBUNTU_IMAGE_LIVECD_ROOTFS_AUTO_PATH")
	if autoSrc == "" {
		dpkgArgs := "dpkg -L livecd-rootfs | grep \"auto$\""
		dpkgCommand := *exec.Command("bash", "-c", dpkgArgs)
		dpkgBytes, err := dpkgCommand.Output()
		if err != nil {
			return err
		}
		autoSrc = strings.TrimSpace(string(dpkgBytes))
	}
	autoDst := rootfs + "/auto"
	if err := osutil.CopySpecialFile(autoSrc, autoDst); err != nil {
		return fmt.Errorf("Error copying livecd-rootfs/auto: %s", err.Error())
	}

	saveCWD := SaveCWD()
	defer saveCWD()
	os.Chdir(rootfs)

	// set up "lb config" and "lb build" commands
	lbConfig := *exec.Command("sudo")
	lbBuild := *exec.Command("sudo")

	lbConfig.Env = append(os.Environ(), env...)
	lbConfig.Args = append(lbConfig.Args, env...)
	lbConfig.Args = append(lbConfig.Args, []string{"lb", "config"}...)
	lbConfig.Stdout = os.Stdout
	lbConfig.Stderr = os.Stderr

	lbBuild.Env = append(os.Environ(), env...)
	lbBuild.Args = append(lbBuild.Args, env...)
	lbBuild.Args = append(lbBuild.Args, []string{"lb", "build"}...)
	lbBuild.Stdout = os.Stdout
	lbBuild.Stderr = os.Stderr

	if arch != GetHostArch() && enableCrossBuild {
		// For cases where we want to cross-build, we need to pass
		// additional options to live-build with the arch to use and path
		// to the qemu static
		var qemuPath string
		qemuPath = os.Getenv("UBUNTU_IMAGE_QEMU_USER_STATIC_PATH")
		if qemuPath == "" {
			static := getQemuStaticForArch(arch)
			tmpQemuPath, err := exec.LookPath(static)
			if err != nil {
				return fmt.Errorf("Use UBUNTU_IMAGE_QEMU_USER_STATIC_PATH in " +
					"case of non-standard archs or custom paths")
			}
			qemuPath = tmpQemuPath
		}
		qemuArgs := []string{"--bootstrap-qemu-arch", arch, "--bootstrap-qemu-static",
			qemuPath, "--architectures", arch}
		lbConfig.Args = append(lbConfig.Args, qemuArgs...)
	}

	// now run the "lb config" and "lb build" commands
	fmt.Println(lbConfig)
	if err := lbConfig.Run(); err != nil {
		return err
	}

	fmt.Println(lbBuild)
	if err := lbBuild.Run(); err != nil {
		return err
	}
	return nil
}

// GetHostSuite checks the release name of the host system to use as a default if --suite is not passed
func GetHostSuite() string {
	cmd := exec.Command("lsb_release", "-c", "-s")
	outputBytes, _ := cmd.Output()
	return strings.TrimSpace(string(outputBytes))
}

// SaveCWD gets the current working directory and returns a function to go back to it
func SaveCWD() func() {
	wd, _ := os.Getwd()
	return func() {
		os.Chdir(wd)
	}
}
