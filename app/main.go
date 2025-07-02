package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/codecrafters-io/bittorrent-starter-go/internal"
)

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345.

func main() {
	// You can use print statements as follows for debugging, they'll be visible
	// when running tests.
	fmt.Fprintln(os.Stderr, "Logs from your program will appear here!")

	command := os.Args[1]

	if command == "decode" {
		// Uncomment this block to pass the first stage
		//
		bencodedValue := os.Args[2]

		decoded, err := internal.DecodeBencode(bencodedValue)
		if err != nil {
			fmt.Println(err)

			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
