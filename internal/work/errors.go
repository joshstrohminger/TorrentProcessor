package work

import "fmt"

// ErrParse indicates the Filepath failed to parse.
type ErrParse struct {
	Err      error
	Filepath string
}

func (e ErrParse) Error() string {
	return fmt.Errorf("failed to parse '%s': %w", e.Filepath, e.Err).Error()
}

// ErrIgnored indicates there was an error and the Filepath was ignored.
type ErrIgnored struct {
	Err      error
	FileName string
}

func (e ErrIgnored) Error() string {
	if e.Err != nil {
		return fmt.Errorf("ignored '%s': %w", e.FileName, e.Err).Error()
	}
	return fmt.Sprintf("ignored '%s'", e.FileName)
}
