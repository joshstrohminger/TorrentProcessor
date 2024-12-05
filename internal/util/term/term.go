package term

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"

	goterm "golang.org/x/term"
)

func PromptYesNo(message string) (bool, error) {
	reader := bufio.NewReader(os.Stdin) // Create a reader for user input

	for {
		fmt.Printf("%s (y/n): ", message)
		input, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(strings.ToLower(input))
		switch input {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Println("Please answer with 'y' or 'n'.")
		}
	}
}

func IsInteractive() bool {
	return goterm.IsTerminal(int(os.Stdin.Fd())) && goterm.IsTerminal(int(os.Stdout.Fd()))
}

func PrintStruct(s any) {
	v := reflect.ValueOf(s)
	t := v.Type()

	if t.Kind() != reflect.Struct {
		fmt.Println(s)
		return
	}

	var maxLen int
	items := make([][2]string, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Name + ":"
		maxLen = max(maxLen, len(name))
		value := v.Field(i).Interface()
		items[i] = [2]string{name, fmt.Sprintf("%v", value)}
	}
	maxLen++

	for _, item := range items {
		fmt.Printf("%*s %s\n", -maxLen, item[0], item[1])
	}
}

func GetEditor() string {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor, _ = exec.LookPath("vim")
	}
	if editor == "" {
		editor, _ = exec.LookPath("vi")
	}
	return editor
}
