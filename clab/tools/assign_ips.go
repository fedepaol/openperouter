package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: assign_ips <input_file>")
		os.Exit(1)
	}

	inputFile := os.Args[1]

	file, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReader(file))
	for {
		record, err := reader.Read()
		if err != nil {
			break
		}

		if len(record) < 3 || strings.HasPrefix(record[0], "#") {
			continue
		}

		containerName := record[0]
		interfaceName := record[1]
		ipAddress := record[2]

		fmt.Printf("Assigning IP %s to interface %s in container %s...\n", ipAddress, interfaceName, containerName)

		cmdAdd := exec.Command("docker", "exec", containerName, "ip", "addr", "add", ipAddress, "dev", interfaceName)
		if err := cmdAdd.Run(); err != nil {
			fmt.Printf("Error assigning IP: %v \n", err)
			continue
		}

		cmdUp := exec.Command("docker", "exec", containerName, "ip", "link", "set", interfaceName, "up")
		if err := cmdUp.Run(); err != nil {
			fmt.Printf("Error bringing interface up: %v\n", err)
			continue
		}
	}
}
