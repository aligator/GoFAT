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
	_ = fs.readRoot()
	return fs
}

func (fs *Fs) readDir(cluster FatEntry) ([]Directory, error) {
	directories := make([]Directory, 0)

	currentCluster := cluster
	for {
		nextCluster := fs.getFatEntry(currentCluster)

		firstSectorOfCluster := ((currentCluster.Value() - 2) * uint32(fs.info.SectorsPerCluster)) + fs.info.FirstDataSector

		for i := uint8(0); i < fs.info.SectorsPerCluster; i++ {
			fs.fetch(firstSectorOfCluster + uint32(i))
			newDirectories := make([]Directory, fs.info.BytesPerSector/32)
			err := binary.Read(bytes.NewReader(fs.sector.buffer), binary.LittleEndian, &newDirectories)
			if err != nil {
				return nil, err
			}

			for _, d := range newDirectories {
				if d != (Directory{}) {
					directories = append(directories, d)
				}
			}
		}

		if nextCluster.ReadAsNextCluster() {
			currentCluster = nextCluster
		} else {
			break
		}

	}

	return directories, nil
}

func (fs *Fs) readRoot() error {
	if fs.info.FSType == FAT12 {
		panic("not supported")
	}

	switch fs.info.FSType {
	case FAT16:
		panic("implement me")
	case FAT32:
		root, err := fs.readDir(fs.info.fat32Specific.RootCluster)
		if err != nil {
			return err
		}

		for _, d := range root {
			fmt.Println(string(d.Name[:]), d.Attr)
		}
	}

	return nil
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

type FatEntry uint32

func (e FatEntry) Value() uint32 {
	return uint32(e)
}

// IsFree only returns true if the sector is unused.
func (e FatEntry) IsFree() bool {
	return (e & 0x0FFFFFFF) == 0x00000000
}

// IsReservedTemp is a special value used to mark clusters as tmp-eof e.g. while writing data to it.
// It should be treated like EOF. Use ReadAsEOF to check for all EOF-like values.
func (e FatEntry) IsReservedTemp() bool {
	return (e & 0x0FFFFFFF) == 0x00000001
}

// IsNextCluster is true if the cluster is a normal data cluster.
// Use ReadAsNextCluster to check for all DataCluster-like values.
func (e FatEntry) IsNextCluster() bool {
	masked := e & 0x0FFFFFFF
	return masked >= 0x00000002 && masked <= 0x0FFFFFEF
}

// IsReservedSometimes is a special value which may occur in rare cases. Should be treated as a DataCluster.
// TODO: For FAT12 a special case exists -> 0xFF0 should be read as EOF. This is not implemented yet.
// Use ReadAsNextCluster to check for all DataCluster-like values.
func (e FatEntry) IsReservedSometimes() bool {
	masked := e & 0x0FFFFFFF
	return masked >= 0x0FFFFFF0 && masked <= 0x0FFFFFF5
}

// IsReserved is a special value which may occur in rare cases. Should be treated as a DataCluster.
// Use ReadAsNextCluster to check for all DataCluster-like values.
func (e FatEntry) IsReserved() bool {
	return (e & 0x0FFFFFFF) == 0x0FFFFFF6
}

// IsBad is a special value which indicates a bad sector. Should be treated as a DataCluster.
// Use ReadAsNextCluster to check for all DataCluster-like values.
func (e FatEntry) IsBad() bool {
	return (e & 0x0FFFFFFF) == 0x0FFFFFF7
}

// IsEOF is a special value used to mark clusters as EOF.
// Use ReadAsEOF to check for all EOF-like values.
func (e FatEntry) IsEOF() bool {
	masked := e & 0x0FFFFFFF
	return masked >= 0x0FFFFFF8 && masked <= 0x0FFFFFFF
}

// ReadAsNextCluster treats all values specified as "should be used as Data Cluster" in
// https://en.wikipedia.org/wiki/Design_of_the_FAT_file_system#Cluster_values
// Use this tho check if it should be read as a normal data cluster.
func (e FatEntry) ReadAsNextCluster() bool {
	// TODO: e.IsReservedSometimes(): MS-DOS/PC DOS 3.3 and higher treats a value of 0xFF0[nb 11][13] on FAT12 (but not on FAT16 or FAT32)
	//       volumes as additional end-of-chain marker similar to 0xFF8-0xFFF.[13] For compatibility with MS-DOS/PC DOS,
	//       file systems should avoid to use data cluster 0xFF0 in cluster chains on FAT12 volumes (that is, treat it
	//       as a reserved cluster similar to 0xFF7). (NB. The correspondence of the low byte of the cluster number with
	//       the FAT ID and media descriptor values is the reason, why these cluster values are reserved.)

	return e.IsNextCluster() || e.IsReservedSometimes() || e.IsReserved() || e.IsBad()
}

// ReadAsEOF treats all values specified as "should be read as EOF" in
// https://en.wikipedia.org/wiki/Design_of_the_FAT_file_system#Cluster_values
// Use this tho check if it should be read as an EOF.
func (e FatEntry) ReadAsEOF() bool {
	return e.IsEOF() || e.IsReservedTemp()
}

// getFatEntry returns the next fat entry for the given cluster.
func (fs *Fs) getFatEntry(cluster FatEntry) FatEntry {
	if fs.info.FSType == FAT12 {
		panic("not supported")
	}

	var fatOffset uint32
	switch fs.info.FSType {
	case FAT16:
		fatOffset = cluster.Value() * 2
	case FAT32:
		fatOffset = cluster.Value() * 4
	}

	fatSectorNumber := uint32(fs.info.ReservedSectorCount) + (fatOffset / uint32(fs.info.BytesPerSector))
	fatEntryOffset := fatOffset % uint32(fs.info.BytesPerSector)
	// TODO: avoid fetch to avoid setting a new sector to fs.sector
	fs.fetch(fatSectorNumber)

	switch fs.info.FSType {
	case FAT16:
		fat16ClusterEntryValue := binary.LittleEndian.Uint16(fs.sector.buffer[fatEntryOffset : fatEntryOffset+2])

		// convert the special values to FAT32 special values
		if fat16ClusterEntryValue >= 0xFFF0 && fat16ClusterEntryValue <= 0xFFFF {
			return FatEntry(uint32(fat16ClusterEntryValue) | 0x0FFFF000&0x0FFFFFFF)
		}

		return FatEntry(fat16ClusterEntryValue)
	case FAT32:
		fat32ClusterEntryValue := binary.LittleEndian.Uint32(fs.sector.buffer[fatEntryOffset:fatEntryOffset+4]) & 0x0FFFFFFF
		return FatEntry(fat32ClusterEntryValue)
	}

	panic("not supported")
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
