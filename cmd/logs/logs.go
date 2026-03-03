package logs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// NewLogsCommand creates the 'logs' subcommand for viewing log entries
func NewLogsCommand() *cobra.Command {
	var tailLines int

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent log entries",
		Long:  "Display the last N log entries from the application log file.",
		Run: func(cmd *cobra.Command, args []string) {
			// Get the log file path
			logFile := getLogFilePath()
			DisplayLogs(logFile, tailLines)
		},
	}

	cmd.Flags().IntVar(&tailLines, "tail", 50,
		"Number of log lines to show (default: 50)")

	return cmd
}

// getLogFilePath returns the path to the application log file
func getLogFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	logDir := filepath.Join(homeDir, ".config", "plexmusic-tui", "logs")
	return filepath.Join(logDir, "app.log")
}

// DisplayLogs reads and displays the last N lines from the log file
func DisplayLogs(logFile string, tailLines int) {
	file, err := os.Open(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not open log file at %s\n", logFile)
		fmt.Fprintf(os.Stderr, "Details: %v\n", err)
		return
	}
	defer file.Close()

	// Read all lines from the file
	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading log file: %v\n", err)
		return
	}

	// Determine which lines to display
	startIdx := 0
	if len(lines) > tailLines {
		startIdx = len(lines) - tailLines
		fmt.Fprintf(os.Stderr, "Showing last %d lines. Full logs at: %s\n\n",
			tailLines, logFile)
	}

	// Print the tail lines
	for i := startIdx; i < len(lines); i++ {
		fmt.Println(lines[i])
	}

	if len(lines) == 0 {
		fmt.Fprintf(os.Stderr, "No log entries found in %s\n", logFile)
	}
}
