package main

import (
	"fmt"
	"os"

	"github.com/aligator/gofat"
)

func main() {
	argsWithoutProg := os.Args[1:]
	if len(argsWithoutProg) <= 0 {
		fmt.Println("Please provide a filename.")
		os.Exit(1)
	}

	fsFile, err := os.Open(argsWithoutProg[0])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer fsFile.Close()

	fat := gofat.New(fsFile)
	file, err := fat.Open("/hallo/")
	if err != nil {
		fmt.Println("Could not open the root file.", err)
		os.Exit(1)
	}

	content, err := file.Readdirnames(0)
	if err != nil {
		fmt.Println("Could get folder content.", err)
		os.Exit(1)
	}

	fmt.Println(content)
}
