package term

import (
	"bufio"
	"fmt"
	"os"
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
