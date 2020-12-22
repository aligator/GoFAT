package gofat

import (
	"bytes"
	"encoding/binary"
	"errors"
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

type Flags struct {
	Dirty       bool
	Open        bool
	SizeChanged bool
	Root        bool
}

// Info contains all information about the whole filesystem.
type Info struct {
	FSType              uint8
	SectorsPerCluster   uint8
	FirstDataSector     uint32
	TotalSectorCount    uint32
	ReservedSectorCount uint16
	BytesPerSector      uint16
	Label               string
	RootDirectory       Directory
	fat32Specific       FAT32SpecificData
	fat16Specific       FAT16SpecificData
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
	fs.readRoot()
	return fs
}

func (fs *Fs) readRoot() {
	if fs.info.FSType == FAT12 {
		panic("not supported")
	}

	switch fs.info.FSType {
	case FAT16:
		panic("implement me")
	case FAT32:
		firstSectorOfCluster := ((fs.info.fat32Specific.RootCluster - 2) * uint32(fs.info.SectorsPerCluster)) + fs.info.FirstDataSector
		fs.fetch(firstSectorOfCluster)

		fmt.Printf("%x\n", fs.sector.buffer)
	}
}

func (fs *Fs) initialize() error {
	fs.reader.Seek(0, io.SeekStart)
	// The data for the first sector is always in the first 512 so use that until the correct sector size is loaded.
	// Note that almost all FAT filesystems use 512.
	// Some may use 1024, 2048 or 4096 but this is not supported by many drivers.
	fs.info.BytesPerSector = 512
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

	// Check if it is really a FAT filesystem.
	// Check for valid jump instructions
	if !(bpb.BSJumpBoot[0] == 0xEB && bpb.BSJumpBoot[2] == 0x90) && !(bpb.BSJumpBoot[0] == 0xE9) {
		return errors.New("no valid jump instructions at the beginning")
	}

	// Load the sector size and use it for all following sector reads.
	// Also FAT only supports 512, 1024, 2048 and 4096
	if bpb.BytesPerSector != 512 && bpb.BytesPerSector != 1024 && bpb.BytesPerSector != 2048 && bpb.BytesPerSector != 4096 {
		return errors.New("invalid sector size")
	}

	// Sectors per cluster has to be a power of two and greater than 0.
	// Also the whole cluster size should not be more than 32K.
	if bpb.SectorsPerCluster%2 != 0 || bpb.SectorsPerCluster == 0 || (bpb.BytesPerSector*uint16(bpb.SectorsPerCluster)) > (32*1024) {
		return errors.New("invalid sectors per cluster")
	}

	// The reserved sector count should not be 0.
	// Note: for FAT12 and FAT16 it is typically 1 for FAT32 it is typically 32.
	if bpb.ReservedSectorCount == 0 {
		return errors.New("invalid reserved sector count")
	}

	// TODO: add check for NumFATs >= 1 and support also 1?

	if bpb.Media != 0xF0 &&
		!(bpb.Media >= 0xF8 && bpb.Media <= 0xFF) {
		return errors.New("invalid media value")
	}

	if fs.sector.buffer[510] != 0x55 || fs.sector.buffer[511] != 0xAA {
		return errors.New("invalid signature at offset 510 / 511")
	}

	var fatSize, totalSectors, dataSectors, countOfClusters uint32

	// Calculate the cluster count to determine the FAT type.
	var rootDirSectors uint32 = ((uint32(bpb.RootEntryCount) * 32) + (uint32(bpb.BytesPerSector) - 1)) / uint32(bpb.BytesPerSector)

	err = binary.Read(bytes.NewReader(bpb.FATSpecificData[:]), binary.LittleEndian, &fs.info.fat32Specific)
	if err != nil {
		return err
	}

	if bpb.FATSize16 != 0 {
		fatSize = uint32(bpb.FATSize16)
	} else {
		fatSize = fs.info.fat32Specific.FatSize
	}

	if bpb.TotalSectors16 != 0 {
		totalSectors = uint32(bpb.TotalSectors16)
	} else {
		totalSectors = bpb.TotalSectors32
	}

	dataSectors = totalSectors - (uint32(bpb.ReservedSectorCount) + uint32(bpb.NumFATs)) + rootDirSectors
	countOfClusters = dataSectors / uint32(bpb.SectorsPerCluster)

	// Now the correct type can be determined based on the cluster count.
	if countOfClusters < 4085 {
		fmt.Println("found FAT12")
		// For now do not support FAT12 as its a bit more complicated.
		return errors.New("FAT12 is not supported")
	} else if countOfClusters < 65525 {
		fmt.Println("found FAT16")
		fs.info.FSType = FAT16
	} else {
		fmt.Println("found FAT32")
		fs.info.FSType = FAT32
	}

	// The root entry count has to be 0 for FAT32 and has to fit exactly into the sectors.
	if fs.info.FSType == FAT32 && bpb.RootEntryCount != 0 || (fs.info.FSType != FAT32 && (bpb.RootEntryCount*32)%bpb.BytesPerSector != 0) {
		return errors.New("invalid root entry count")
	}

	err = binary.Read(bytes.NewReader(bpb.FATSpecificData[:]), binary.LittleEndian, &fs.info.fat16Specific)
	if err != nil {
		return err
	}

	// Now all needed data can be saved. See FAT spec for details.
	fs.info.BytesPerSector = bpb.BytesPerSector
	if bpb.TotalSectors16 != 0 {
		fs.info.TotalSectorCount = uint32(bpb.TotalSectors16)
	} else {
		fs.info.TotalSectorCount = bpb.TotalSectors32
	}
	dataSectors = fs.info.TotalSectorCount - (uint32(bpb.ReservedSectorCount) + (uint32(bpb.NumFATs) * fatSize) + rootDirSectors)
	fs.info.SectorsPerCluster = bpb.SectorsPerCluster
	fs.info.ReservedSectorCount = bpb.ReservedSectorCount
	fs.info.FirstDataSector = uint32(bpb.ReservedSectorCount) + (uint32(bpb.NumFATs) * fatSize) + rootDirSectors

	if fs.info.FSType == FAT32 {
		fs.info.Label = string(fs.info.fat32Specific.BSVolumeLabel[:])
	} else {
		fs.info.Label = string(fs.info.fat16Specific.BSVolumeLabel[:])
	}

	fmt.Printf("found volume \"%v\"\n", fs.info.Label)

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
	_, err := fs.reader.Seek(int64(sector)*int64(fs.info.BytesPerSector), io.SeekStart)
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

func (fs *Fs) fetchFatEntry(cluster uint32) {
	if fs.info.FSType == FAT12 {
		panic("not supported")
	}

	var fatOffset uint32
	switch fs.info.FSType {
	case FAT16:
		fatOffset = cluster * 2
	case FAT32:
		fatOffset = cluster * 4
	}

	fatSectorNumber := uint32(fs.info.ReservedSectorCount) + (fatOffset / uint32(fs.info.BytesPerSector))
	fatEntryOffset := fatOffset % uint32(fs.info.BytesPerSector)
	fs.fetch(fatSectorNumber)

	switch fs.info.FSType {
	case FAT16:
		fat16ClusterEntryValue := binary.LittleEndian.Uint16(fs.sector.buffer[fatEntryOffset : fatEntryOffset+2])
		fmt.Printf("%x\n", fat16ClusterEntryValue)
	case FAT32:
		fat32ClusterEntryValue := binary.LittleEndian.Uint32(fs.sector.buffer[fatEntryOffset:fatEntryOffset+4]) & 0x0FFFFFFF
		fmt.Printf("%x\n", fat32ClusterEntryValue)
	}
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
	return "FAT"
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
