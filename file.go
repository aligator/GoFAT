package gofat

import (
	"os"
	"syscall"
)

type File struct {
	fs   *Fs
	path string

	isDirectory bool
	isReadOnly  bool
	isHidden    bool
	isSystem    bool

	firstCluster fatEntry
	size         uint32
	stat         os.FileInfo
}

func (f File) Close() error {
	f.fs = nil
	f.path = ""
	f.isDirectory = false
	f.isReadOnly = false
	f.isHidden = false
	f.isSystem = false
	f.firstCluster = 0
	f.size = 0
	f.stat = nil

	return nil
}

func (f File) Read(p []byte) (n int, err error) {
	data, err := f.fs.readFile(f.firstCluster, len(p))
	if err != nil {
		return 0, err
	}

	copy(p, data)
	return len(data), nil
}

func (f File) ReadAt(p []byte, off int64) (n int, err error) {
	data, err := f.fs.readFileAt(f.firstCluster, off, len(p))
	if err != nil {
		return 0, err
	}

	copy(p, data)
	return len(data), nil
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
	return f.stat.Name()
}

func (f File) Readdir(count int) ([]os.FileInfo, error) {
	if !f.isDirectory {
		return nil, syscall.ENOTDIR
	}

	var content []ExtendedEntryHeader
	var err error
	if f.path == "/" {
		content, err = f.fs.readRoot()
	} else {
		content, err = f.fs.readDir(f.firstCluster)
	}

	if err != nil {
		return nil, err
	}

	// TODO: Maybe support the count param directly in readDir to avoid reading too much.
	//       (Not sure if that's easy and worth it)
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
	return f.stat, nil
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
