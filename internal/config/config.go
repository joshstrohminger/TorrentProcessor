package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const Ext = ".yaml"

type App struct {
	WorkPath        string        `yaml:"workPath"`
	MovieOutputPath string        `yaml:"movieOutputPath"`
	TvOutputPath    string        `yaml:"tvOutputPath"`
	ContentPath     string        `yaml:"contentPath"`
	LogPath         string        `yaml:"logPath"`
	DormantPeriod   time.Duration `yaml:"dormantPeriod"`
	MaxRetries      int           `yaml:"maxRetries"`
	Api             Api           `yaml:"api"`
}

func (cfg App) Marshal() ([]byte, error) {
	return yaml.Marshal(cfg)
}

type Api struct {
	Host string `yaml:"host"`
	Port uint16 `yaml:"port"`
}

func (a Api) String() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}

type Process struct {
	App
	DryRun bool
	Limit  int
}

// Validate that fields ending in 'Path' and not 'OutputPath' exist. If an output path is a network
// drive that disconnected
func (a App) Validate() error {
	var errs []error

	_ = a.VisitPaths(func(name, path string) error {
		if _, err := os.Stat(path); err != nil {
			// don't add errors for "OutputPath" fields that don't exist, but continue to Stat them to prompt for permissions if needed
			if !errors.Is(err, fs.ErrNotExist) || !strings.HasSuffix(name, "OutputPath") {
				errs = append(errs, fmt.Errorf("%s doesn't exist: %s", name, path))
			}
		}
		return nil
	})

	return errors.Join(errs...)
}

// VisitorFunc will be called with the field name and the path value for that field. Return an error to end early with that error.
type VisitorFunc func(name string, path string) error

func (a App) VisitPaths(f VisitorFunc) error {
	v := reflect.ValueOf(a)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		name := t.Field(i).Name
		if field.Kind() == reflect.String && strings.HasSuffix(name, "Path") {
			if err := f(name, field.String()); err != nil {
				return err
			}
		}
	}

	return nil
}
