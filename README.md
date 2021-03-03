[![Actions Status](https://github.com/aligator/gofat/workflows/build/badge.svg)](https://github.com/aligator/gofat/actions) ![CodeQL](https://github.com/aligator/GoFAT/workflows/CodeQL/badge.svg) [![codecov](https://codecov.io/gh/aligator/GoFAT/branch/main/graph/badge.svg?token=EUUUT368Z0)](https://codecov.io/gh/aligator/GoFAT)
# GoFAT

A FAT filesystem implementation in pure Go.  
It implements the interface of [afero](https://github.com/spf13/afero).

## Current Status

Readonly File access works great. Write support is missing, yet.

## Usage

```go
package main

import (
	"fmt"
	"io"
	"os"
	"github.com/aligator/gofat"
	"github.com/spf13/afero"
)

func main() {
	// Get any io.ReadSeeker implementation which provides a reader to a FAT32 filesystem.
	// You can use for example os.Open(...) to open an image file or even a `/dev/sdxy` device file from linux. 
	var reader io.ReadSeeker = ...

	// Then create a new GoFAT filesystem from the reader.
	fat, err := gofat.New(reader)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Now you can access any method from the afero.Fs filesystem like for example afero.Walk.
	_ := afero.Walk(fat, "/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return err
		}
		fmt.Println(path, info.IsDir())
		return nil
	})

	// You can also access some FAT specific fields.
	fmt.Printf("Opened volume '%v' with type %v\n\n", fat.Label(), fat.FSType())
}
```

That's it!

## Compatibility with Go 1.16

As the Go 1.16 fs.FS interface is not fully compatible with the afero.Fs interface, it cannot be used with that directly.
But I added a simple wrapper around it.  
You can either just wrap an existing fat fs: 
```go
gofs := GoFs{*fs}
```

Or directly create a new one using `NewGoFS(...)` or `NewGoFSSkipChecks(...)`.  
Note that this wrapper has a small overhead, especially ReadDir because the result has to be converted to `[]fs.DirEntry`.

I also added `testing.fstest` to the unit tests.

## Test images

To get access to some test-images which already contain a FAT filesystem just run

```bash
go generate
```

This will extract the test images into `./testdata`.

## Contribution

Contributions are welcome, just create issues or even better PRs. You may open a draft or issue first to discuss the
changes you would like to implement.

You may use github.com/cweill/gotests to generate your test boilerplate code.

Some resources on how FAT works are:

* https://en.wikipedia.org/wiki/Design_of_the_FAT_file_system
* https://wiki.osdev.org/FAT
* https://github.com/ryansturmer/thinfat32
* https://github.com/ryansturmer/thinfat32/blob/master/fatgen103.pdf

## ToDo

* more tests
* implement some more attributes (e.g. hidden)
* implement write support
* support FAT12
* check if compatibility with TinyGo for microcontrollers is possible. That would be a good use case for this lib...
* use for a fuse filesystem driver
