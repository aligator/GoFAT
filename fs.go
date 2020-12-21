package gofat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/afero"
)

const (
	FAT12 = iota
	FAT16 = iota
	FAT32 = iota
)

type SectorSize uint16

type Flags struct {
	Dirty       bool
	Open        bool
	SizeChanged bool
	Root        bool
}

// Info contains all information about the whole filesystem.
type Info struct {
	FSType            uint8
	SectorsPerCluster uint8
	FirstDataSector   uint32
	TotalSectors      uint32
	ReservedSectors   uint16
	SectorSize        uint16
	rootDirectorySize uint32
}

type BPB struct {
	BSJumpBoot          [3]byte
	BSOEMName           [8]byte
	BytesPerSector      uint16
	SectorsPerCluster   byte
	ReservedSectorCount uint16
	NumFATs             byte
	RootEntryCount      uint16
	TotalSectors16      uint16
	Media               byte
	FATSize16           uint16
	SectorsPerTrack     uint16
	NumberOfHeads       uint16
	HiddenSectors       uint32
	TotalSectors32      uint32
	FATSpecificData     [54]byte
}

type Sector struct {
	current uint32
	flags   Flags
	buffer  []uint8
}

type Fs struct {
	reader io.ReadSeeker
	info   Info
	sector Sector
}

func New(reader io.ReadSeeker) afero.Fs {
	fs := &Fs{
		reader: reader,
	}

	fs.initialize()

	return fs
}

func (fs *Fs) initialize() error {
	fs.reader.Seek(0, io.SeekStart)
	// The data for the first sector is always in the first 512 so use that until the correct sector size is loaded.
	// Note that almost all FAT filesystems use 512.
	// Some may use 1024, 2048 or 4096 but this is not supported by many drivers.
	fs.info.SectorSize = 512
	fs.sector.buffer = make([]uint8, 512)

	// Read sec0
	// Set to a sector unequal 0 to avoid using empty buffer in fetch.
	fs.sector.current = 0xFFFFFFFF
	fs.fetch(0)

	// Read sector as BPB
	bpb := BPB{}
	err := binary.Read(bytes.NewReader(fs.sector.buffer), binary.LittleEndian, &bpb)
	if err != nil {
		return err
	}

	fmt.Println(bpb)

	// Check if it is really a FAT filesystem.
	// Check for valid jump instructions
	if !(bpb.BSJumpBoot[0] == 0xEB && bpb.BSJumpBoot[2] == 0x90) && !(bpb.BSJumpBoot[0] == 0xE9) {
		return fmt.Errorf("no valid jump instructions at the beginning")
	}

	// Load the sector size and use it for all following sector reads.
	// Also FAT only supports 512, 1024, 2048 and 4096
	if bpb.BytesPerSector != 512 && bpb.BytesPerSector != 1024 && bpb.BytesPerSector != 2048 && bpb.BytesPerSector != 4096 {
		return fmt.Errorf("invalid sector size")
	}
	fs.info.SectorSize = bpb.BytesPerSector

	// Sectors per cluster has to be a power of two and greater than 0.
	// Also the whole cluster size should not be more than 32K.
	if bpb.SectorsPerCluster%2 != 0 || bpb.SectorsPerCluster == 0 || (bpb.BytesPerSector*uint16(bpb.SectorsPerCluster)) > (32*1024) {
		return fmt.Errorf("invalid sectors per cluster")
	}

	// The reserved sector count should not be 0.
	// Note: for FAT12 and FAT16 it is typically 1 for FAT32 it is typically 32.
	if bpb.ReservedSectorCount == 0 {
		return fmt.Errorf("invalid reserved sector count")
	}
	fs.info.ReservedSectors = bpb.ReservedSectorCount

	// TODO: add check for NumFATs >= 1 and support also 1?

	if bpb.Media != 0xF0 ||
		bpb.Media != 0xF8 ||
		bpb.Media != 0xF9 ||
		bpb.Media != 0xFA ||
		bpb.Media != 0xFB ||
		bpb.Media != 0xFC ||
		bpb.Media != 0xFD ||
		bpb.Media != 0xFE ||
		bpb.Media != 0xFF {
		return fmt.Errorf("invalid media value")
	}

	//TODO if type = FAT32 && bpb.RootEntryCount != 0 || (type != FAT32 && (bpb.RootEntryCount * 32) % bpb.BytesPerSec != 0)

	//TODO if type = FAT32 && bpb.TotalSectors16 != 0
	//TODO if type = FAT32 && bpb.TotalSectors32 == 0
	//TODO if type != FAT32 && (bpb.TotalSectors16 == 0 && bpb.TotalSectors32 != 0) && (bpb.TotalSectors32 == 0 && bpb.TotalSectors16 != 0)
	if bpb.TotalSectors16 != 0 {
		fs.info.TotalSectors = uint32(bpb.TotalSectors16)
	} else if bpb.TotalSectors32 != 0 {
		fs.info.TotalSectors = bpb.TotalSectors32
	}

	//TODO if type = FAT32 && bpb.FATSize16 != 0 && bpb.FATSize32 == 0 || (type != FAT32 && bpb.FATSiz16 == 0 && bpb.FATSiz32 != 0)
	//if bpb.FATSize16 != 0 {
	//	fs.info.FATSize = bpb.FATSize16
	//} else if bpb.FATSize32 != 0 {
	//	fs.info.FATSize = bpb.FATSize32
	//}

	return nil
}

// fetch loads a specific single sector of the filesystem.
func (fs *Fs) fetch(sector uint32) error {
	// Only load it once.
	if sector == fs.sector.current {
		return nil
	}

	// If already fetched sector is dirty, write it
	if fs.sector.flags.Dirty {
		err := fs.store()
		if err != nil {
			return err
		}
	}

	// Seek to and Read the new sector.
	_, err := fs.reader.Seek(int64(sector)*int64(fs.info.SectorSize), io.SeekStart)
	if err != nil {
		return err
	}

	_, err = fs.reader.Read(fs.sector.buffer)
	if err != nil {
		return err
	}

	fs.sector.current = sector

	return nil
}

func (fs *Fs) store() error {
	panic("implement me")
}

func (fs *Fs) Create(name string) (afero.File, error) {
	panic("implement me")
}

func (fs *Fs) Mkdir(name string, perm os.FileMode) error {
	panic("implement me")
}

func (fs *Fs) MkdirAll(path string, perm os.FileMode) error {
	panic("implement me")
}

func (fs *Fs) Open(name string) (afero.File, error) {
	panic("implement me")
}

func (fs *Fs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	panic("implement me")
}

func (fs *Fs) Remove(name string) error {
	panic("implement me")
}

func (fs *Fs) RemoveAll(path string) error {
	panic("implement me")
}

func (fs *Fs) Rename(oldname, newname string) error {
	panic("implement me")
}

func (fs *Fs) Stat(name string) (os.FileInfo, error) {
	panic("implement me")
}

func (fs *Fs) Name() string {
	panic("implement me")
}

func (fs *Fs) Chmod(name string, mode os.FileMode) error {
	panic("implement me")
}

func (fs *Fs) Chown(name string, uid, gid int) error {
	panic("implement me")
}

func (fs *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	panic("implement me")
}
