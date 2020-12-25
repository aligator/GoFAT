package gofat

import "os"

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
	if f.path != "/" {
		panic("implement me")
	}

	content, err := f.fs.readRoot()
	if err != nil {
		return nil, err
	}

	// TODO: Maybe support the count param directly in readRoot to avoid reading too much.
	//       (Not that easy though because of e.g. the long file names.)
	if count > 0 {
		content = content[:count]
	}

	return content, nil
}

func (f File) Readdirnames(n int) ([]string, error) {
	content, err := f.Readdir(n)
	if err != nil {
		return nil, err
	}

	names := make([]string, n)
	for _, entry := range content {
		names = append(names, entry.Name())
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
