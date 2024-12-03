package util

import "fmt"

func GetMapValue[T any](m map[string]any, key string) (value T, err error) {
	rawValue, ok := m[key]
	if !ok {
		err = fmt.Errorf("missing key: %s", key)
		return
	}

	value, ok = rawValue.(T)
	if !ok {
		err = fmt.Errorf("value of key %s type should be %T: %T", key, value, rawValue)
	}

	return
}
