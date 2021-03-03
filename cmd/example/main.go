package main

import (
	"fmt"
	"github.com/aligator/gofat"
	"github.com/spf13/afero"
	"io"
	"os"
)

// main is just a example main to play with GoFAT.
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

	fat, err := gofat.New(fsFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Printf("Opened volume '%v' with type %v\n\n", fat.Label(), fat.FSType())

	afero.Walk(fat, "", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return err
		}
		fmt.Println(path, info.IsDir(), info.ModTime())
		return nil
	})

	file, err := fat.Open("README.md")
	if err != nil {
		fmt.Println("could not open the root file", err)
		os.Exit(1)
	}

	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		fmt.Println("could not stat the file", err)
		os.Exit(1)
	}
	buffer := make([]byte, stat.Size())
	n, err := file.Read(buffer)
	if err != nil {
		fmt.Println("could not read the file", err)
		os.Exit(1)
	}
	fmt.Println(stat.Size(), n)
	fmt.Println("\n\nContent of " + stat.Name() + ":\n\n" + string(buffer))

	buffer = make([]byte, 52)
	offset, err := file.Seek(9, io.SeekStart)
	if err != nil {
		fmt.Println("could not seek", err)
		os.Exit(1)
	}

	fmt.Println(offset, err)
	offset, err = file.Seek(52*199, io.SeekCurrent)
	if err != nil {
		fmt.Println("could not seek", err)
		os.Exit(1)
	}
	fmt.Println(offset, err)

	n, err = file.Read(buffer)
	if err != nil {
		fmt.Println("could not read the file", err)
		os.Exit(1)
	}
	fmt.Println(stat.Size(), n)
	fmt.Println("\n\nContent of " + stat.Name() + " using an offset and small buffer:\n\n" + string(buffer))
}
