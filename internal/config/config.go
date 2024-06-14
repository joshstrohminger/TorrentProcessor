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
	WorkPath        string        `yaml:"work_path,omitempty"`
	MovieOutputPath string        `yaml:"movie_output_path,omitempty"`
	TvOutputPath    string        `yaml:"tv_output_path,omitempty"`
	DormantPeriod   time.Duration `yaml:"dormant_period,omitempty"`
	MaxRetries      int           `yaml:"max_retries,omitempty"`
	Api             Api           `yaml:"api,omitempty"`
}

type Api struct {
	Host string `yaml:"host,omitempty"`
	Port uint16 `yaml:"port,omitempty"`
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
			p := field.String()
			if _, err := os.Stat(p); err != nil {
				errs = append(errs, fmt.Errorf("%s doesn't exist: %s", name, p))
			}
		}
	}

	return errors.Join(errs...)
}
