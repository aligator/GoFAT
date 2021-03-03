package gofat

import (
	"errors"
	"fmt"
	"github.com/aligator/gofat/checkpoint"
	"io"
	"os"
	"syscall"

	"github.com/spf13/afero"
)

// These errors may occur while processing a file.
var (
	ErrReadFile = errors.New("could not read file completely")
	ErrSeekFile = errors.New("could not seek inside of the file")
	ErrReadDir  = errors.New("could not read the directory")
)

// fatFileFs provides all methods needed from a fat filesystem for File.
// It mainly exists to be able to mock the Fs in tests.
// Generated mock using mockgen:
//  mockgen -source=file.go -destination=file_mock.go -package gofat
type fatFileFs interface {
	readFileAt(cluster fatEntry, fileSize int64, offset int64, readSize int64) ([]byte, error)
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
	if p == nil {
		return 0, nil
	}

	// Reading a file if the size has been already reached, makes no sense.
	if f.stat.Size() <= f.offset {
		return 0, io.EOF
	}

	offset := f.offset
	data, err := f.fs.readFileAt(f.firstCluster, f.stat.Size(), offset, int64(len(p)))

	if data != nil {
		copy(p, data)
	}

	// Seek even if an error occurred, errors from reading are used even if seek also errors.
	_, seekErr := f.Seek(int64(len(data)), io.SeekCurrent)

	if err != nil {
		return len(data), checkpoint.Wrap(err, ErrReadFile)
	}

	if seekErr != nil {
		return len(data), checkpoint.Wrap(seekErr, ErrReadFile)
	}

	return len(data), nil
}

func (f *File) ReadAt(p []byte, off int64) (n int, err error) {
	if p == nil {
		return 0, nil
	}

	// Reading over the end makes no sense.
	if f.stat.Size() <= off {
		return 0, io.EOF
	}

	size := len(p)
	data, err := f.fs.readFileAt(f.firstCluster, f.stat.Size(), off, int64(size))

	if data != nil {
		copy(p, data)
	}

	if err != nil {
		return len(data), checkpoint.Wrap(err, ErrReadFile)
	}

	if len(data) < size {
		return len(data), checkpoint.Wrap(err, ErrReadFile)
	}
	return len(data), nil
}

// Seek jumps to a specific offset in the file. This affects all Read operation except ReadAt.
// May return a syscall.EINVAL error if the whence value is invalid.
// May return an afero.ErrOutOfRange error if the offset is out of range.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset = f.offset + offset
	case io.SeekEnd:
		offset = f.stat.Size() + offset
	default:
		return 0, checkpoint.Wrap(ErrSeekFile, fmt.Errorf("%w, offset: %v, whence: %v", syscall.EINVAL, offset, whence))
	}

	if offset < 0 || offset > f.stat.Size() {
		return 0, checkpoint.Wrap(afero.ErrOutOfRange, fmt.Errorf("%w, offset: %v, whence: %v", ErrSeekFile, offset, whence))
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

// Readdir reads the contents of a directory.
// May return syscall.ENOTDIR if the current File is no directory.
func (f *File) Readdir(count int) ([]os.FileInfo, error) {
	if !f.isDirectory {
		return nil, checkpoint.Wrap(syscall.ENOTDIR, ErrReadDir)
	}

	var content []ExtendedEntryHeader
	var err error
	if f.path == "" {
		content, err = f.fs.readRoot()
	} else {
		content, err = f.fs.readDir(f.firstCluster)
	}

	if err != nil {
		return nil, checkpoint.Wrap(err, ErrReadDir)
	}

	end := len(content)

	if int64(len(content)) < f.offset+int64(count) {
		count = len(content) - int(f.offset)
		err = io.EOF
	}

	if count >= 0 {
		end = int(f.offset) + count
	}

	// TODO: Maybe support the count param directly in readDir to avoid reading too much.
	//       (Not sure if that's easy and worth it)
	content = content[f.offset:end]

	if count > 0 {
		f.offset += int64(count)
	} else if count < 0 {
		f.offset = int64(end)
	}

	result := make([]os.FileInfo, len(content))
	for i := range content {
		result[i] = content[i].FileInfo()
	}

	return result, err
}

func (f *File) Readdirnames(count int) ([]string, error) {
	content, err := f.Readdir(count)
	if err != nil {
		return nil, checkpoint.Wrap(err, ErrReadDir)
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
