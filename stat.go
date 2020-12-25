package gofat

import (
	"os"
	"time"
)

// TODO: Support long file names.

func (h *EntryHeader) FileInfo() os.FileInfo {
	return entryHeaderFileInfo{*h}
}

type entryHeaderFileInfo struct {
	entry EntryHeader
}

func (e entryHeaderFileInfo) Name() string {
	return string(e.entry.Name[:])
}

func (e entryHeaderFileInfo) Size() int64 {
	return int64(e.entry.FileSize)
}

func (e entryHeaderFileInfo) Mode() os.FileMode {
	if e.IsDir() {
		return os.ModeDir
	}
	return 0
}

func (e entryHeaderFileInfo) ModTime() time.Time {
	panic("implement me")
}

func (e entryHeaderFileInfo) IsDir() bool {
	return e.entry.Attr&0x10 == 0x10
}

func (e entryHeaderFileInfo) Sys() interface{} {
	return e.entry
}