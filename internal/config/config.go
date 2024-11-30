package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"
)

type App struct {
	WorkPath        string        `yaml:"workPath"`
	MovieOutputPath string        `yaml:"movieOutputPath"`
	TvOutputPath    string        `yaml:"tvOutputPath"`
	LogPath         string        `yaml:"logPath"`
	DormantPeriod   time.Duration `yaml:"dormantPeriod"`
	MaxRetries      int           `yaml:"maxRetries"`
	Api             Api           `yaml:"api"`
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

func (a App) Validate() error {
	var errs []error

	v := reflect.ValueOf(a)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		name := t.Field(i).Name
		if strings.HasSuffix(name, "Path") && field.Kind() == reflect.String {
			path := field.String()
			if _, err := os.Stat(path); err != nil {
				errs = append(errs, fmt.Errorf("%s doesn't exist: %s", name, path))
			}
		}
	}

	return errors.Join(errs...)
}
