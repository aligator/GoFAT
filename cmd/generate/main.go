package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// main for extracting the test images. Can be executed using 'go generate' from the project root.
func main() {
	src := "testdata/testdata.zip"
	dest := "testdata"

	r, err := zip.OpenReader(src)
	if err != nil {
		panic(err)
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: https://snyk.io/research/zip-slip-vulnerability#go
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			panic(fmt.Errorf("%s: illegal file path", fpath))
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			panic(err)
		}

		rc, err := f.Open()
		if err != nil {
			panic(err)
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()

		if err != nil {
			panic(err)
		}
	}
}
