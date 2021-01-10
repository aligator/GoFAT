package gofat

import (
	"os"
	"strings"
	"time"
)

func (h *ExtendedEntryHeader) FileInfo() os.FileInfo {
	return entryHeaderFileInfo{*h}
}

type entryHeaderFileInfo struct {
	entry ExtendedEntryHeader
}

func (e entryHeaderFileInfo) Name() string {
	if e.entry.ExtendedName != "" {
		return e.entry.ExtendedName
	}

	name := strings.TrimRight(string(e.entry.Name[:8]), " ")
	ext := strings.TrimRight(string(e.entry.Name[8:11]), " ")

	if ext != "" {
		name += "."
	}

	return name + ext
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
	writeDate := ParseDate(e.entry.WriteDate)
	writeTime := ParseTime(e.entry.WriteTime)

	// If the date IsZero() it contained any invalid value in which case we return time.Time{}.
	// For writeTime we cannot do that because writeTime.IsZero() is perfectly valid.
	if writeDate.IsZero() {
		return time.Time{}
	}

	return time.Date(writeDate.Year(), writeDate.Month(), writeDate.Day(), writeTime.Hour(), writeTime.Minute(), writeTime.Second(), 0, time.UTC)
}

func (e entryHeaderFileInfo) IsDir() bool {
	return e.entry.Attribute&0x10 == 0x10
}

func (e entryHeaderFileInfo) Sys() interface{} {
	return e.entry
}
