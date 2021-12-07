package container

import (
	"fmt"
	"os/exec"
	"path"
	"strings"
)

// SelectEngine check what container engine is present on the machine.
func SelectEngine() (*exec.Cmd, error) {
	for _, containerEngine := range []string{"docker", "podman"} {
		containerEngineCMD := exec.Command(containerEngine)
		directoryName, _ := path.Split(containerEngineCMD.Path)

		if directoryName != "" {
			return containerEngineCMD, nil
		}
	}

	return nil, fmt.Errorf("no container Engine present on host machine")
}

// PullImage pulls image from registry to a local machine.
func PullImage(imageRegistry string, imageName string) error {
	containerEngine, err := SelectEngine()
	if err != nil {
		return err
	}

	if strings.Contains(containerEngine.Path, "docker") {
		err := validateDockerDaemonRunning()
		if err != nil {
			return err
		}
	}

	status := exec.Command(
		containerEngine.Path,
		"pull",
		fmt.Sprintf("%s/%s", imageRegistry, imageName))
	_, err = status.Output()

	if err != nil {
		return err
	}

	status = exec.Command(
		containerEngine.Path,
		"images", "--quiet",
		fmt.Sprintf("%s/%s", imageRegistry, imageName))

	commandOutput, err := status.Output()
	if err != nil {
		return err
	}

	if string(commandOutput) == "" {
		return fmt.Errorf("error to pull the image")
	}

	return nil
}

func validateDockerDaemonRunning() error {
	isDaemonRunning := exec.Command("systemctl", "is-active", "--quiet", "docker")
	if isDaemonRunning.Run() != nil {
		return fmt.Errorf("docker daemon is not active on host")
	}

	return nil
}
