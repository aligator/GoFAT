package gofat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type File struct {
	fs   *Fs
	path string

	isDirectory bool
	isReadOnly  bool
	isHidden    bool
	isSystem    bool

	firstCluster uint32
	size         uint32
}

func (f File) Close() error {
	panic("implement me")
}

func (f File) Read(p []byte) (n int, err error) {
	panic("implement me")
}

func (f File) ReadAt(p []byte, off int64) (n int, err error) {
	panic("implement me")
}

func (f File) Seek(offset int64, whence int) (int64, error) {
	panic("implement me")
}

func (f File) Write(p []byte) (n int, err error) {
	panic("implement me")
}

func (f File) WriteAt(p []byte, off int64) (n int, err error) {
	panic("implement me")
}

func (f File) Name() string {
	panic("implement me")
}

func (f File) Readdir(count int) ([]os.FileInfo, error) {
	if !f.isDirectory {
		return nil, syscall.ENOTDIR
	}

	content, err := f.fs.readRoot()
	if err != nil {
		return nil, err
	}

	path := filepath.ToSlash(f.path)
	pathParts := strings.Split(path, "/")

	// Go through the path until the last pathPart and then use the contents of that folder as result.
pathLoop:
	for _, pathPart := range pathParts {
		if pathPart == "" {
			continue
		}

		for _, entry := range content {
			fileInfo := entry.FileInfo()
			// Note: FAT is not case sensitive.
			if strings.ToUpper(strings.Trim(fileInfo.Name(), " ")) == strings.ToUpper(pathPart) {
				if !fileInfo.IsDir() {
					return nil, syscall.ENOTDIR
				}

				content, err = f.fs.readDir(fatEntry(uint32(entry.FirstClusterHI)<<16 | uint32(entry.FirstClusterLO)))
				if err != nil {
					return nil, err
				}

				continue pathLoop
			}
		}
		return nil, errors.New("path doesn't exist")
	}

	// TODO: Maybe support the count param directly in readRoot to avoid reading too much.
	//       (Not that easy though because of e.g. we still have to load the whole path)
	if count > 0 {
		content = content[:count]
	}

	result := make([]os.FileInfo, len(content))
	for i := range content {
		result[i] = content[i].FileInfo()
	}

	return result, nil
}

func (f File) Readdirnames(n int) ([]string, error) {
	content, err := f.Readdir(n)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(content))
	for i, entry := range content {
		names[i] = entry.Name()
	}

	return names, nil
}

func (f File) Stat() (os.FileInfo, error) {
	panic("implement me")
}

func (f File) Sync() error {
	panic("implement me")
}

func (f File) Truncate(size int64) error {
	panic("implement me")
}

func (f File) WriteString(s string) (ret int, err error) {
	panic("implement me")
}
