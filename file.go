package gofat

import (
	"io"
	"os"
	"syscall"

	"github.com/spf13/afero"
)

// fatFileFs provides all methods needed from a fat filesystem for File.
// It mainly exists to be able to mock the Fs in tests.
type fatFileFs interface {
	readFileAt(cluster fatEntry, offset int64, size int) ([]byte, error)
	readRoot() ([]ExtendedEntryHeader, error)
	readDir(cluster fatEntry) ([]ExtendedEntryHeader, error)
}

type File struct {
	fs   fatFileFs
	path string

	isDirectory bool
	isReadOnly  bool
	isHidden    bool
	isSystem    bool

	firstCluster fatEntry
	stat         os.FileInfo
	offset       int64
}

func (f *File) Close() error {
	f.fs = nil
	f.path = ""
	f.isDirectory = false
	f.isReadOnly = false
	f.isHidden = false
	f.isSystem = false
	f.firstCluster = 0
	f.stat = nil
	f.offset = 0

	return nil
}

func (f *File) Read(p []byte) (n int, err error) {
	data, err := f.fs.readFileAt(f.firstCluster, f.offset, len(p))
	if err != nil {
		return len(data), err
	}

	copy(p, data)
	return len(data), nil
}

func (f *File) ReadAt(p []byte, off int64) (n int, err error) {
	size := len(p)
	data, err := f.fs.readFileAt(f.firstCluster, off, size)
	if err != nil {
		return len(data), err
	}

	copy(p, data)
	if len(data) < size {
		return len(data), io.EOF
	}
	return len(data), nil
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset = f.offset + offset
	case io.SeekEnd:
		offset = f.stat.Size() - offset
	default:
		return 0, syscall.EINVAL
	}

	if offset < 0 || offset > f.stat.Size() {
		return 0, afero.ErrOutOfRange
	}

	f.offset = offset
	return offset, nil
}

func (f *File) Write(p []byte) (n int, err error) {
	panic("implement me")
}

func (f *File) WriteAt(p []byte, off int64) (n int, err error) {
	panic("implement me")
}

func (f *File) Name() string {
	return f.stat.Name()
}

func (f *File) Readdir(count int) ([]os.FileInfo, error) {
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

func (f *File) Readdirnames(n int) ([]string, error) {
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

func (f *File) Stat() (os.FileInfo, error) {
	return f.stat, nil
}

func (f *File) Sync() error {
	panic("implement me")
}

func (f *File) Truncate(size int64) error {
	panic("implement me")
}

func (f *File) WriteString(s string) (ret int, err error) {
	return f.Write([]byte(s))
}
