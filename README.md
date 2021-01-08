# GoFAT
A FAT filesystem implementation in Go.  
It implements the interface of [afero](https://github.com/spf13/afero).

## Current Status
Readonly File access works great.
Write support is missing, yet.

## Usage
```go
// Get any io.ReadSeeker implementation which provides a reader to a FAT32 filesystem.
// You can use for example os.Open(...) to open an image file or even a `/dev/sdxy` device file from linux. 
var reader io.ReadSeeker = ...

// Then create a new gofat filesystem from the reader
fat, err := gofat.New(reader)
if err != nil {
    fmt.Println(err)
    os.Exit(1)
}

// Now you can access any method from the afero.Fs filesystem like for example afero.Walk
afero.Walk(fat, "/", func(path string, info os.FileInfo, err error) error {
    if err != nil {
        fmt.Println(err)
        return err
    }
    fmt.Println(path, info.IsDir())
    return nil
})

// You can also access some FAT specific fields.
fmt.Printf("Opened volume '%v' with type %v\n\n", fat.Label(), fat.FSType())
```

That's it!

## Test images
To get access to some test-images which already contain a FAT filesystem just run
```bash
go generate
```
This will extract the test images into `./testdata`.

## Contribution
Contributions are welcome, just create issues or even better PRs.
You may open a draft or issue first to discuss the changes you would like to implement.

You may use github.com/cweill/gotests to generate your test boilerplate code.

Some resources on how FAT works are:
* https://en.wikipedia.org/wiki/Design_of_the_FAT_file_system
* https://docs.microsoft.com/en-us/windows/win32/fileio/exfat-specification
* https://wiki.osdev.org/FAT
* https://github.com/ryansturmer/thinfat32
* https://github.com/ryansturmer/thinfat32/blob/master/fatgen103.pdf

## ToDo
* More tests + CI
* implement dates
* implement some more attributes (e.g. hidden)
* long filenames need some more validation according to the specs (e.g. checksum, ...)
* implement /write support
* support FAT12
* Check if compatibility with TinyGo for microcontrollers is possible. That would be a good use case for this lib...
* Use for a fuse filesystem driver
