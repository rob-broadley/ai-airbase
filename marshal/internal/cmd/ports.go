// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"
)

// validatePort returns an error if port is outside the non-privileged range (1024-65535).
func validatePort(port int) error {
	if port < 1024 || port > 65535 {
		return fmt.Errorf("invalid port %d: must be in range 1024-65535 (ports below 1024 are privileged and blocked)", port)
	}
	return nil
}

// getProjectPorts returns a map of port number to project name for all other projects.
func getProjectPorts(deps Deps, currentProject string) (map[int]string, error) {
	projects, _, err := deps.listProjects()()
	if err != nil {
		return nil, err
	}
	portToProject := make(map[int]string)
	for _, otherProj := range projects {
		if otherProj == currentProject {
			continue
		}
		otherCfg, err := deps.loadConfig()(otherProj)
		if err != nil {
			continue
		}
		otherPort := otherCfg.Port
		if otherPort == 0 {
			otherPort = 4096
		}
		portToProject[otherPort] = otherProj
	}
	return portToProject, nil
}

// isPortConfigured checks if another project has the given port configured in its TOML file.
// Returns the name of the conflicting project if found, or empty string.
func isPortConfigured(deps Deps, currentProject string, port int) (string, error) {
	portToProject, err := getProjectPorts(deps, currentProject)
	if err != nil {
		return "", err
	}
	if otherProj, exists := portToProject[port]; exists {
		return otherProj, nil
	}
	return "", nil
}

// findFreePort scans ports starting at 4096 to find the first port that is not configured
// in another project and is not currently bound on the host loopback.
func findFreePort(deps Deps, currentProject string) (int, error) {
	portToProject, err := getProjectPorts(deps, currentProject)
	if err != nil {
		return 0, fmt.Errorf("checking project port conflicts: %w", err)
	}

	for port := 4096; port <= 65535; port++ {
		if _, exists := portToProject[port]; exists {
			continue
		}
		if deps.isPortBound()(port) {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("no free ports available in range 4096-65535")
}

// resolveAndValidatePort handles port selection, validation, and collision checks.
func resolveAndValidatePort(deps Deps, project string, cfgPort, portFlag int) (int, error) {
	resolvedPort := portFlag
	if resolvedPort == 0 && cfgPort != 0 {
		resolvedPort = cfgPort
	}
	if resolvedPort != 0 {
		if err := validatePort(resolvedPort); err != nil {
			return 0, err
		}
		conflictProj, err := isPortConfigured(deps, project, resolvedPort)
		if err != nil {
			return 0, err
		}
		if conflictProj != "" {
			return 0, fmt.Errorf("port %d is already configured for project %q", resolvedPort, conflictProj)
		}
		if deps.isPortBound()(resolvedPort) {
			return 0, fmt.Errorf("port %d is already in use on the host", resolvedPort)
		}
	} else {
		var err error
		resolvedPort, err = findFreePort(deps, project)
		if err != nil {
			return 0, err
		}
	}
	return resolvedPort, nil
}
