package provider

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
)

// ProviderConfig represents configuration parameters for a model host provider.
// This structure holds all details necessary for SSH connections and GCP instance mapping.
// It is mapped via Viper configuration loading from workspace properties.
// Instances represent remote endpoints running AI model endpoints.
type ProviderConfig struct {
	Type       string `mapstructure:"type"`
	Project    string `mapstructure:"project"`
	Zone       string `mapstructure:"zone"`
	Instance   string `mapstructure:"instance"`
	SSHUser    string `mapstructure:"ssh_user"`
	SSHKey     string `mapstructure:"ssh_key"`
	LocalPort  int    `mapstructure:"local_port"`
	RemotePort int    `mapstructure:"remote_port"`
}

// GetIP queries the GCP compute engine instance NAT IP address.
func GetIP(dbPath string, cfg ProviderConfig) (string, error) {
	out, err := nomosexec.RunCommand(dbPath, "", "gcloud", "compute", "instances", "describe", cfg.Instance,
		"--zone="+cfg.Zone,
		"--project="+cfg.Project,
		"--format=value(networkInterfaces[0].accessConfigs[0].natIP)")

	if err != nil {
		return "", fmt.Errorf("failed to get NAT IP: %w (output: %s)", err, out)
	}

	ip := strings.TrimSpace(out)
	if ip == "" {
		return "", errors.New("instance does not have an external NAT IP assigned")
	}

	return ip, nil
}

// StatusProvider retrieves the current operational status of the compute engine instance.
func StatusProvider(dbPath string, cfg ProviderConfig) (string, error) {
	out, err := nomosexec.RunCommand(dbPath, "", "gcloud", "compute", "instances", "describe", cfg.Instance,
		"--zone="+cfg.Zone,
		"--project="+cfg.Project,
		"--format=value(status)")

	if err != nil {
		return "", fmt.Errorf("failed to query status: %w (output: %s)", err, out)
	}

	return strings.TrimSpace(out), nil
}

// StartProvider triggers instance boot and blocks until the instance is RUNNING.
func StartProvider(dbPath string, cfg ProviderConfig) (string, error) {
	status, err := StatusProvider(dbPath, cfg)
	if err == nil && status == "RUNNING" {
		return GetIP(dbPath, cfg)
	}

	// Trigger instance boot
	out, err := nomosexec.RunCommand(dbPath, "", "gcloud", "compute", "instances", "start", cfg.Instance,
		"--zone="+cfg.Zone,
		"--project="+cfg.Project)

	if err != nil {
		return "", fmt.Errorf("failed to start instance: %w (output: %s)", err, out)
	}

	// Wait loop for instance status readiness (up to 90 seconds)
	for i := 0; i < 18; i++ {
		time.Sleep(5 * time.Second)
		status, err := StatusProvider(dbPath, cfg)
		if err == nil && status == "RUNNING" {
			return GetIP(dbPath, cfg)
		}
	}

	return "", errors.New("timeout waiting for compute instance to enter RUNNING state")
}

// StopProvider gracefully shuts down the GCP compute engine instance to reduce billing charges.
func StopProvider(dbPath string, cfg ProviderConfig) error {
	out, err := nomosexec.RunCommand(dbPath, "", "gcloud", "compute", "instances", "stop", cfg.Instance,
		"--zone="+cfg.Zone,
		"--project="+cfg.Project)

	if err != nil {
		return fmt.Errorf("failed to stop instance: %w (output: %s)", err, out)
	}

	return nil
}

// StartTunnel opens an SSH dynamic tunnel in the background using the workspace substrate command execution wrapper.
func StartTunnel(dbPath string, cfg ProviderConfig, ip string) (*os.Process, error) {
	keyPath := workspace.ExpandHomePath(cfg.SSHKey)
	dest := fmt.Sprintf("%s@%s", cfg.SSHUser, ip)
	forwardRule := fmt.Sprintf("%d:localhost:%d", cfg.LocalPort, cfg.RemotePort)

	// Build ssh tunnel execution arguments
	args := []string{
		"-N",
		"-L", forwardRule,
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "ExitOnForwardFailure=yes",
		dest,
	}

	return nomosexec.StartCommand(dbPath, "", "ssh", args...)
}

// CheckLocalPort checks if the configured local port is currently open/reachable.
func CheckLocalPort(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err == nil {
		conn.Close()
		return true
	}
	return false
}
