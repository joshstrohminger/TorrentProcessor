package daemon

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"text/template"

	"github.com/joshstrohminger/TorrentProcessor/internal/app"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
)

//go:embed daemon.plist.tmpl
var plistTemplate string

type TemplateData struct {
	Exe     string
	Info    Info
	Config  config.App
	Version string
	Name    string
}

type Info struct {
	Label        string
	Domain       string
	Name         string
	Path         string
	DebugLogName string
	Conflict     bool
	Installed    bool
	Enabled      bool
	Running      bool
	Queued       int
}

func InstallPlist(info Info, cfg config.App, version string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse plist template: %w", err)
	}

	file, err := os.OpenFile(info.Path, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open %s for write: %w", info.Path, err)
	}
	defer file.Close()

	if err = tmpl.Execute(file, TemplateData{exe, info, cfg, version, app.LongName}); err != nil {
		return fmt.Errorf("failed to execute plist template: %w", err)
	}

	return nil
}

func GetInfo() (Info, error) {
	var info Info
	var err error

	info.Label, err = getLabel()
	if err != nil {
		return info, err
	}
	label := info.Label + ".process"

	home, err := os.UserHomeDir()
	if err != nil {
		return info, fmt.Errorf("failed to get user home directory: %w", err)
	}

	info.Path = filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	if _, err = os.Stat(info.Path); err == nil {
		info.Installed = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		// check if something with the same name is installed
		out, err := exec.Command("launchctl", "list").CombinedOutput()
		if err != nil {
			return info, fmt.Errorf("failed to list existing daemons: %w", err)
		}

		info.Conflict, err = regexp.Match(`\s`+label+`(\s|$)`, out)
		if err != nil {
			return info, fmt.Errorf("failed to check for existing daemons: %w", err)
		}

		return info, nil
	}

	id, err := getUserId()
	if err != nil {
		return info, err
	}
	info.Domain = fmt.Sprintf("gui/%s", id)
	info.Name = fmt.Sprintf("%s/%s", info.Domain, label)

	if info.Installed {
		info.Enabled, info.Running, err = getStatus(info.Name)
		if err != nil {
			return info, err
		}
	}

	return info, nil
}

func getStatus(name string) (enabled bool, running bool, err error) {
	out, err := exec.Command("launchctl", "print", name).CombinedOutput()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 113 {
			// not enabled
			err = nil
			return
		}

		err = fmt.Errorf("failed to get daemon %s status: %w", name, err)
		return
	}

	enabled = true

	reg, err := regexp.Compile(`\sstate = ((:?not )?running)\s`)
	if err != nil {
		err = fmt.Errorf("failed to get daemon %s status: failed to compile regex: %w", name, err)
		return
	}

	output := string(out)
	matches := reg.FindStringSubmatch(output)
	if matches == nil {
		err = fmt.Errorf("failed to find state for daemon %s", name)
		return
	}
	state := matches[1]

	switch state {
	case "running":
		running = true
	case "not running":
	default:
		err = fmt.Errorf("unhandled state in daemon %s: %s", name, state)
		return
	}

	return
}

func getLabel() (string, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", fmt.Errorf("failed to get daemon label: failed to get build info")
	}

	parts := strings.Split(info.Path, "/")

	domainParts := strings.Split(parts[0], ".")
	slices.Reverse(domainParts)
	parts[0] = strings.Join(domainParts, ".")

	return strings.Join(parts, "."), nil
}

func getUserId() (string, error) {
	// lookup the user name in case we're running as sudo
	user, ok := os.LookupEnv("SUDO_USER")
	if !ok {
		user, ok = os.LookupEnv("USER")
		if !ok {
			return "", fmt.Errorf("failed to lookup current user via environment variables")
		}
	}

	out, err := exec.Command("id", "-u", user).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get user ID: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
