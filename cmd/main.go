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

	file, err := os.Open(argsWithoutProg[0])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer file.Close()

	fs := gofat.New(file)

	fmt.Println(fs)
}
