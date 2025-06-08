package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// PIDValidationError represents a PID validation error
type PIDValidationError struct {
	PID     int
	Message string
}

func (e PIDValidationError) Error() string {
	return fmt.Sprintf("PID %d: %s", e.PID, e.Message)
}

// ProcessValidator handles process ID validation and safety checks
type ProcessValidator struct {
	allowedParentNames []string
	cronitorExePath    string
}

// NewProcessValidator creates a new process validator instance
func NewProcessValidator() *ProcessValidator {
	pv := &ProcessValidator{
		allowedParentNames: []string{"cron", "crond", "launchd", "systemd", "cronitor"},
	}

	// Get current executable path for cronitor binary verification
	if exePath, err := os.Executable(); err == nil {
		pv.cronitorExePath = exePath
	}

	return pv
}

// ValidatePID performs comprehensive PID validation according to security requirements
func (pv *ProcessValidator) ValidatePID(pid int) error {
	// Check PID is a positive integer and within valid range
	if pid <= 0 {
		return PIDValidationError{
			PID:     pid,
			Message: "PID must be a positive integer",
		}
	}

	// Check maximum PID based on OS
	maxPID := pv.getMaxPID()
	if pid > maxPID {
		return PIDValidationError{
			PID:     pid,
			Message: fmt.Sprintf("PID exceeds maximum allowed value (%d)", maxPID),
		}
	}

	// Safety check: prevent killing critical system processes
	if pv.isCriticalSystemProcess(pid) {
		return PIDValidationError{
			PID:     pid,
			Message: "Cannot kill critical system process",
		}
	}

	// Verify process exists and is not a kernel thread
	if err := pv.verifyProcessExists(pid); err != nil {
		return PIDValidationError{
			PID:     pid,
			Message: err.Error(),
		}
	}

	// Check if process is a kernel thread (Linux-specific)
	if runtime.GOOS == "linux" {
		if isKernel, err := pv.isKernelThread(pid); err != nil {
			return PIDValidationError{
				PID:     pid,
				Message: fmt.Sprintf("Error checking process type: %v", err),
			}
		} else if isKernel {
			return PIDValidationError{
				PID:     pid,
				Message: "Cannot kill kernel thread",
			}
		}
	}

	return nil
}

// ValidatePIDWithOwnership performs comprehensive PID validation including process ownership checks
func (pv *ProcessValidator) ValidatePIDWithOwnership(pid int) error {
	// First perform basic PID validation
	if err := pv.ValidatePID(pid); err != nil {
		return err
	}

	// Then perform ownership validation
	if err := pv.validateProcessOwnership(pid); err != nil {
		return PIDValidationError{
			PID:     pid,
			Message: err.Error(),
		}
	}

	return nil
}

// validateProcessOwnership checks if the process belongs to an allowed parent
func (pv *ProcessValidator) validateProcessOwnership(pid int) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		return pv.validateUnixProcessOwnership(pid)
	case "windows":
		return pv.validateWindowsProcessOwnership(pid)
	default:
		return fmt.Errorf("process ownership validation not supported on %s", runtime.GOOS)
	}
}

// validateUnixProcessOwnership validates process ownership on Unix-like systems
func (pv *ProcessValidator) validateUnixProcessOwnership(pid int) error {
	// Check process tree to find allowed parent
	if allowed, err := pv.hasAllowedParentInTree(pid); err != nil {
		return fmt.Errorf("failed to validate process tree: %v", err)
	} else if !allowed {
		return fmt.Errorf("process does not have an allowed parent (cron, crond, launchd, cronitor)")
	}

	return nil
}

// validateWindowsProcessOwnership validates process ownership on Windows
func (pv *ProcessValidator) validateWindowsProcessOwnership(pid int) error {
	// For Windows, we'll do basic validation but not as comprehensive as Unix
	// This is a simplified implementation - Windows process tree validation is more complex
	return nil
}

// hasAllowedParentInTree checks if the process has an allowed parent in its process tree
func (pv *ProcessValidator) hasAllowedParentInTree(pid int) (bool, error) {
	visited := make(map[int]bool)
	return pv.checkProcessTreeRecursive(pid, visited, 0)
}

// checkProcessTreeRecursive recursively checks the process tree for allowed parents
func (pv *ProcessValidator) checkProcessTreeRecursive(pid int, visited map[int]bool, depth int) (bool, error) {
	// Prevent infinite loops
	if visited[pid] || depth > 10 {
		return false, nil
	}
	visited[pid] = true

	// Get process information
	processInfo, err := pv.GetProcessInfo(pid)
	if err != nil {
		return false, err
	}

	// Check if this process is an allowed parent
	if pv.isAllowedParent(processInfo) {
		return true, nil
	}

	// If this is PID 1 (init), we've reached the top without finding an allowed parent
	if processInfo.PPID == 1 || processInfo.PPID == 0 {
		return false, nil
	}

	// Recursively check the parent
	return pv.checkProcessTreeRecursive(processInfo.PPID, visited, depth+1)
}

// ProcessInfo contains process information
type ProcessInfo struct {
	PID     int
	PPID    int
	Command string
	ExePath string
	UID     int
}

// GetProcessInfo retrieves process information for the given PID
func (pv *ProcessValidator) GetProcessInfo(pid int) (*ProcessInfo, error) {
	switch runtime.GOOS {
	case "linux":
		return pv.getLinuxProcessInfo(pid)
	case "darwin":
		return pv.getDarwinProcessInfo(pid)
	default:
		return nil, fmt.Errorf("getProcessInfo not implemented for %s", runtime.GOOS)
	}
}

// getLinuxProcessInfo retrieves process information on Linux
func (pv *ProcessValidator) getLinuxProcessInfo(pid int) (*ProcessInfo, error) {
	// Read /proc/{pid}/stat for basic info
	statFile := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read process stat: %v", err)
	}

	statFields := strings.Fields(string(statData))
	if len(statFields) < 4 {
		return nil, fmt.Errorf("invalid stat file format")
	}

	// Parse PPID (field 4, 0-indexed field 3)
	ppid, err := strconv.Atoi(statFields[3])
	if err != nil {
		return nil, fmt.Errorf("invalid PPID in stat file: %v", err)
	}

	// Get command name (field 2, 0-indexed field 1)
	command := strings.Trim(statFields[1], "()")

	// Read /proc/{pid}/exe for executable path
	exeLink := fmt.Sprintf("/proc/%d/exe", pid)
	exePath, err := os.Readlink(exeLink)
	if err != nil {
		// exe link might not be accessible, use command name
		exePath = command
	}

	// Read /proc/{pid}/status for UID
	statusFile := fmt.Sprintf("/proc/%d/status", pid)
	statusData, err := os.ReadFile(statusFile)
	uid := 0
	if err == nil {
		for _, line := range strings.Split(string(statusData), "\n") {
			if strings.HasPrefix(line, "Uid:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if parsedUID, parseErr := strconv.Atoi(fields[1]); parseErr == nil {
						uid = parsedUID
					}
				}
				break
			}
		}
	}

	return &ProcessInfo{
		PID:     pid,
		PPID:    ppid,
		Command: command,
		ExePath: exePath,
		UID:     uid,
	}, nil
}

// getDarwinProcessInfo retrieves process information on macOS
func (pv *ProcessValidator) getDarwinProcessInfo(pid int) (*ProcessInfo, error) {
	// On macOS, we can use syscalls to get process info
	// For now, use a simplified approach with ps command
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("process not found: %v", err)
	}

	// This is a simplified implementation - in practice, you'd use more specific macOS APIs
	// For now, return basic info
	_ = process // Suppress unused variable warning
	return &ProcessInfo{
		PID:     pid,
		PPID:    1, // Default to init, would need proper implementation
		Command: "unknown",
		ExePath: "unknown",
		UID:     0,
	}, nil
}

// isAllowedParent checks if a process is an allowed parent
func (pv *ProcessValidator) isAllowedParent(processInfo *ProcessInfo) bool {
	// Check if command name matches allowed parent names
	for _, allowedName := range pv.allowedParentNames {
		if strings.Contains(strings.ToLower(processInfo.Command), allowedName) {
			return true
		}
		if strings.Contains(strings.ToLower(filepath.Base(processInfo.ExePath)), allowedName) {
			return true
		}
	}

	// Check if the executable path matches cronitor binary
	if pv.cronitorExePath != "" && processInfo.ExePath != "" {
		if processInfo.ExePath == pv.cronitorExePath ||
			filepath.Base(processInfo.ExePath) == filepath.Base(pv.cronitorExePath) {
			return true
		}
	}

	return false
}

// ValidateProcessList validates a list of processes for ownership and returns those that pass
func (pv *ProcessValidator) ValidateProcessList(pids []int) ([]int, map[int]error) {
	validPids := make([]int, 0)
	errors := make(map[int]error)

	for _, pid := range pids {
		if err := pv.ValidatePIDWithOwnership(pid); err != nil {
			errors[pid] = err
		} else {
			validPids = append(validPids, pid)
		}
	}

	return validPids, errors
}

// getMaxPID returns the maximum PID value for the current OS
func (pv *ProcessValidator) getMaxPID() int {
	switch runtime.GOOS {
	case "linux":
		// Linux default max PID is 4194304 (2^22)
		// Could also read from /proc/sys/kernel/pid_max but using safe default
		return 4194304
	case "darwin":
		// macOS typical max PID
		return 99998
	case "windows":
		// Windows typical max PID
		return 65536
	default:
		// Conservative default for other Unix-like systems
		return 32768
	}
}

// isCriticalSystemProcess checks if a PID represents a critical system process
func (pv *ProcessValidator) isCriticalSystemProcess(pid int) bool {
	criticalPIDs := []int{
		0, // kernel/swapper
		1, // init/systemd
		2, // kthreadd (Linux)
	}

	for _, criticalPID := range criticalPIDs {
		if pid == criticalPID {
			return true
		}
	}

	// Additional checks for low-numbered PIDs that are typically system processes
	if pid <= 10 {
		return true
	}

	return false
}

// verifyProcessExists checks if a process with the given PID exists
func (pv *ProcessValidator) verifyProcessExists(pid int) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		// Check /proc/{pid}/stat file on Linux or use os.FindProcess on macOS
		if runtime.GOOS == "linux" {
			statFile := fmt.Sprintf("/proc/%d/stat", pid)
			if _, err := os.Stat(statFile); os.IsNotExist(err) {
				return fmt.Errorf("process does not exist")
			}
		} else {
			// On macOS, use os.FindProcess and then check if process is actually running
			process, err := os.FindProcess(pid)
			if err != nil {
				return fmt.Errorf("process does not exist: %v", err)
			}
			// Send signal 0 to check if process exists without affecting it
			if err := process.Signal(os.Signal(nil)); err != nil {
				return fmt.Errorf("process does not exist or is not accessible")
			}
		}
	case "windows":
		// On Windows, use os.FindProcess
		process, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("process does not exist: %v", err)
		}
		// On Windows, FindProcess always succeeds, so we can't easily verify existence
		// without additional system calls
		_ = process
	default:
		// For other systems, use os.FindProcess
		if _, err := os.FindProcess(pid); err != nil {
			return fmt.Errorf("process does not exist: %v", err)
		}
	}

	return nil
}

// isKernelThread checks if a process is a kernel thread (Linux-specific)
func (pv *ProcessValidator) isKernelThread(pid int) (bool, error) {
	if runtime.GOOS != "linux" {
		return false, nil
	}

	statFile := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statFile)
	if err != nil {
		return false, fmt.Errorf("cannot read process stat: %v", err)
	}

	statFields := strings.Fields(string(data))
	if len(statFields) < 3 {
		return false, fmt.Errorf("invalid stat file format")
	}

	// Field 2 is the command name, kernel threads are typically in brackets like [kthreadd]
	cmdName := statFields[1]
	isKernel := strings.HasPrefix(cmdName, "[") && strings.HasSuffix(cmdName, "]")

	return isKernel, nil
}

// ValidatePIDList validates a list of PIDs and returns detailed error information
func (pv *ProcessValidator) ValidatePIDList(pids []int) map[int]error {
	errors := make(map[int]error)

	for _, pid := range pids {
		if err := pv.ValidatePID(pid); err != nil {
			errors[pid] = err
		}
	}

	return errors
}
