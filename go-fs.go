package gofat

import (
	"errors"
	"io"
	"io/fs"
)

type GoDirEntry struct {
	fs.FileInfo
}

func (g GoDirEntry) Type() fs.FileMode {
	return g.FileInfo.Mode().Type()
}

func (g GoDirEntry) Info() (fs.FileInfo, error) {
	// TODO: This may return an error satisfying errors.Is(err, ErrNotExist) if the file does not exist anymore.
	return g.FileInfo, nil
}

type GoFile struct {
	*File
}

func (g GoFile) Stat() (fs.FileInfo, error) {
	return g.File.Stat()
}

func (g GoFile) Read(bytes []byte) (int, error) {
	return g.File.Read(bytes)
}

func (g GoFile) Close() error {
	return g.File.Close()
}

func (g GoFile) ReadDir(n int) ([]fs.DirEntry, error) {
	entries, err := g.File.Readdir(n)

	goEntries := make([]fs.DirEntry, len(entries))
	for i, e := range entries {
		goEntries[i] = GoDirEntry{e}
	}

	return goEntries, err
}

// GoFs just wraps the afero FAT implementation to be compatible with fs.FS.
type GoFs struct {
	Fs
}

// NewGoFS opens a FAT filesystem from the given reader as fs.FS compatible filesystem.
func NewGoFS(reader io.ReadSeeker) (*GoFs, error) {
	fs, err := New(reader)
	if err != nil {
		return nil, err
	}

	return &GoFs{*fs}, nil
}

// NewGoFSSkipChecks opens a FAT filesystem from the given reader as fs.FS compatible filesystem just like NewGoFs but
// it skips some filesystem validations which may allow you to open not perfectly standard FAT filesystems.
// Use with caution!
func NewGoFSSkipChecks(reader io.ReadSeeker) (*GoFs, error) {
	fs, err := NewSkipChecks(reader)
	if err != nil {
		return nil, err
	}

	return &GoFs{*fs}, nil
}

func (g GoFs) Open(name string) (fs.File, error) {
	file, err := g.Fs.Open(name)
	if err != nil {
		return nil, err
	}

	f, ok := file.(*File)
	if !ok {
		return nil, errors.New("invalid File implementation")
	}

	return GoFile{f}, nil
}
