package ranhelper

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// RunMustGather runs must-gather and returns the dir the cmd gets executed from, must-gather output, and error if any.
func RunMustGather() (mustGatherExecDir string, mustGatherOutput []byte, err error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	output, err := ExecAndLogCommand(true, 45*time.Minute, "oc", "adm", "must-gather")
	return dir, output, err
}

// DeleteMustGathers deletes given must-gather dir.
// If mustGatherExecDir is an empty string, then current work directory will be checked.
func DeleteMustGathers(mustGatherExecDir string) error {
	// Look for must-gathers under current dir if mustGatherExecDir is empty
	if mustGatherExecDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return err
		}
		mustGatherExecDir = currentDir
	}

	matches, err := filepath.Glob(fmt.Sprintf("%s/must-gather.local.*", mustGatherExecDir))
	if err != nil {
		return err
	}
	if matches != nil {
		log.Println("Must-gather dirs to be removed:", matches)
	}
	for _, match := range matches {
		// Best effort
		err = os.RemoveAll(match)
	}
	// Returns last error only
	return err
}
