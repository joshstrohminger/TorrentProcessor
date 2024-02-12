package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

type App struct {
	WorkPath        string
	MovieOutputPath string
	TvOutputPath    string
}

type Process struct {
	App
	DryRun     bool
	Limit      int
	MaxRetries int
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
