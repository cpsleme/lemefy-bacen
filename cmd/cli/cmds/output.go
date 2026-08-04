package cmds

import (
	"encoding/json"
	"fmt"
	"os"
)

func outputJSON(data interface{}) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	_, err = os.Stdout.Write(bytes)
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	fmt.Println()
	return nil
}