package gofat

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/aligator/gofat/checkpoint"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/afero"
)

type FATType string

const (
	FAT12 FATType = "FAT12"
	FAT16 FATType = "FAT16"
	FAT32 FATType = "FAT32"
)

const (
	AttrReadOnly  = 0x01
	AttrHidden    = 0x02
	AttrSystem    = 0x04
	AttrVolumeId  = 0x08
	AttrDirectory = 0x10
	AttrArchive   = 0x20
	AttrDevice    = 0x40
	AttrReserved  = 0x80
	AttrLongName  = AttrReadOnly | AttrHidden | AttrSystem | AttrVolumeId
)

// These errors may occur while processing a FAT filesystem.
var (
	ErrInvalidPath          = errors.New("invalid path")
	ErrOpenFilesystem       = errors.New("could not open the filesystem")
	ErrReadFilesystemFile   = errors.New("could not read file completely from the filesystem")
	ErrReadFilesystemDir    = errors.New("could not a directory from the filesystem")
	ErrNotSupported         = errors.New("not supported")
	ErrInitializeFilesystem = errors.New("initialize the filesystem")
	ErrFetchingSector       = errors.New("could not fetch a new sector")
	ErrReadFat              = errors.New("could not read FAT sector")
)

// Info contains all information about the whole filesystem.
type Info struct {
	FSType              FATType
	FatCount            uint8
	FatSize             uint32
	SectorsPerCluster   uint8
	FirstDataSector     uint32
	TotalSectorCount    uint32
	ReservedSectorCount uint16
	BytesPerSector      uint16
	Label               string
	fat32Specific       FAT32SpecificData
	fat16Specific       FAT16SpecificData
	RootEntryCount      uint16 // RootEntryCount is only needed for < FAT32.
}

type Sector struct {
	current uint32
	buffer  []uint8
}

type Fs struct {
	lock        sync.Mutex
	reader      io.ReadSeeker
	info        Info
	sectorCache Sector
}

// New opens a FAT filesystem from the given reader.
func New(reader io.ReadSeeker) (*Fs, error) {
	fs := &Fs{
		reader: reader,
	}

	err := fs.initialize(false)
	if err != nil {
		return nil, checkpoint.Wrap(err, ErrOpenFilesystem)
	}
	return fs, nil
}

// NewSkipChecks opens a FAT filesystem from the given reader just like New but
// it skips some filesystem validations which may allow you to open not perfectly standard FAT filesystems.
// Use with caution!
func NewSkipChecks(reader io.ReadSeeker) (*Fs, error) {
	fs := &Fs{
		reader: reader,
	}

	err := fs.initialize(true)
	if err != nil {
		return nil, checkpoint.Wrap(err, ErrOpenFilesystem)
	}
	return fs, err
}

// readFileAt reads a file which starts at the given cluster but it skips
// the first bytes so that is starts reading at the given offset.
// It only returns max the requested amount of bytes.
// A fileSize of < 0 indicates that it is unknown and therefore it reads until the end of the last sector.
// If readSize is <= 0 it returns the whole file.
// If readSize is > fileSize it also just returns the whole file but also io.EOF as error.
// If an error occurs all bytes read until then and the error is returned. io.EOF is ignored in that case.
func (f *Fs) readFileAt(cluster fatEntry, fileSize int64, offset int64, readSize int64) ([]byte, error) {
	// finalize returns the data sliced to either the readSize, the fileSize or 'as it is'.
	// It may return io.EOF if readSize + offset > fileSize.
	// Use it before any return in readFileAt.
	finalize := func(result []byte, err error) ([]byte, error) {
		if fileSize < 0 {
			fileSize = int64(len(result)) + offset
		}

		if err == nil && readSize > fileSize-offset {
			err = io.EOF
			readSize = fileSize - offset
		}

		// The file was not as long as it should be.
		if err == nil && int64(len(result)) < fileSize-offset && int64(len(result)) < readSize {
			err = io.ErrUnexpectedEOF
		}

		// Return at most the readSize as requested.
		// A readSize of <= 0 means to return till EOF.
		if readSize > 0 && int64(len(result)) > readSize {
			return result[:readSize], checkpoint.Wrap(err, ErrReadFilesystemFile)
		}

		// Return the whole file
		if int64(len(result)) > fileSize {
			return result[:fileSize], checkpoint.Wrap(err, ErrReadFilesystemFile)
		}

		// Else just return the result.
		return result, checkpoint.Wrap(err, ErrReadFilesystemFile)
	}

	data := make([]byte, 0)

	clusterNumber := 0
	currentCluster := cluster

	// Find the cluster to start.
	// We still have to load the cluster number chain.
	for {
		if int64(clusterNumber)*int64(f.info.SectorsPerCluster)*int64(f.info.BytesPerSector) <= offset &&
			int64(clusterNumber+1)*int64(f.info.SectorsPerCluster)*int64(f.info.BytesPerSector) >= offset {
			break
		}

		nextCluster, err := f.getFatEntry(currentCluster)
		if err != nil {
			return finalize(data, err)
		}

		if !nextCluster.ReadAsNextCluster() {
			return finalize(data, nil)
		}

		currentCluster = nextCluster
		clusterNumber++
	}

	// offsetRest contains the offset which is needed for the actual first sector.
	// First the clusters which we already ignored get removed from the offset to initialize the offsetRest.
	offsetRest := offset - (int64(clusterNumber) * int64(f.info.SectorsPerCluster) * int64(f.info.BytesPerSector))

	// Calculate the sectors to skip for the first sector.
	skip := uint8(offsetRest / int64(f.info.BytesPerSector))

	// Calculate the final offsetRest by removing these skipped sectors also from the offsetRest.
	// The result is the value we have to ignore when reading the first sector.
	offsetRest -= int64(f.info.BytesPerSector) * int64(skip)

	// Read the clusters.
	for {
		firstSectorOfCluster := ((currentCluster.Value() - 2) * uint32(f.info.SectorsPerCluster)) + f.info.FirstDataSector

		// Read the sectors of the cluster, skip the first ones if needed
		for i := skip; i < f.info.SectorsPerCluster; i++ {
			sector, err := f.fetch(firstSectorOfCluster + uint32(i))
			if err != nil {
				return finalize(data, err)
			}

			newData := make([]byte, f.info.BytesPerSector)
			err = binary.Read(bytes.NewReader(sector.buffer), binary.LittleEndian, &newData)
			if err != nil {
				return finalize(nil, err)
			}

			// Trim the first bytes based on the offsetRest if it is the first read.
			if len(data) == 0 {
				data = append(data, newData[offsetRest:]...)
				continue
			}

			data = append(data, newData...)
		}

		skip = 0

		// Stop when the size needed is reached.
		if readSize > int64(0) && int64(clusterNumber+1)*int64(f.info.SectorsPerCluster)*int64(f.info.BytesPerSector) >= offset+readSize {
			break
		}

		nextCluster, err := f.getFatEntry(currentCluster)
		if err != nil {
			return finalize(data, err)
		}

		if !nextCluster.ReadAsNextCluster() {
			// The file was not as long as it should be.
			if err == nil && int64(len(data)) < fileSize-offset {
				return finalize(data, io.ErrUnexpectedEOF)
			}
			break
		}

		currentCluster = nextCluster
		clusterNumber++
	}

	return finalize(data, nil)
}

// parseDir reads and interprets a directory-file. It returns a slice of ExtendedEntryHeader,
// one for each file in the directory. It may return an error if it cannot be parsed.
func (f *Fs) parseDir(data []byte) ([]ExtendedEntryHeader, error) {
	entries := make([]EntryHeader, len(data)/32)

	err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &entries)
	if err != nil {
		return nil, checkpoint.Wrap(err, ErrReadFilesystemDir)
	}

	var longFilename []LongFilenameEntry
	var lastLongFilenameIndex = -1

	resetLongFilename := func(i int) {
		longFilename = nil
		lastLongFilenameIndex = i
	}

	// Convert to fatFiles and filter empty entries.
	directory := make([]ExtendedEntryHeader, 0)
	for i, entry := range entries {
		// Check the first byte of the name as it may contain special values.
		// End of FAT
		if entry.Name[0] == 0x00 {
			break
		}

		// Dot-entry (e.g. .. or .) Note that 0x2E is actually a '.'.
		if entry.Name[0] == 0x2E {
			// For now just ignore them. Don't know if we need them for something but
			// afero.Walk cannot cope with it for now.
			continue
		}

		// Deleted Entry
		if entry.Name[0] == 0xE5 {
			continue
		}

		// Initial character is actually 0xE5
		if entry.Name[0] == 0x05 {
			entry.Name[0] = 0xE5
		}

		// Save extended file name parts.
		if entry.Attribute&AttrLongName == AttrLongName {
			// First get the bytes again but only for this one entry.
			entryBytes := data[i*32 : (i+1)*32]

			// Then parse it as LongFilenameEntry.
			longFilenameEntry := LongFilenameEntry{}
			err = binary.Read(bytes.NewReader(entryBytes), binary.LittleEndian, &longFilenameEntry)
			if err != nil {
				return nil, checkpoint.Wrap(err, ErrReadFilesystemDir)
			}

			// Ignore deleted entry.
			if longFilenameEntry.Sequence == 0xE5 {
				continue
			}

			// If the 0x40 bit of the sequence is set, it means that this is the beginning of a long filename.
			// Therefore we need to reset everything before.
			if longFilenameEntry.Sequence&0x40 == 0x40 {
				resetLongFilename(i - 1)
			}

			if lastLongFilenameIndex+1 != i {
				// All long filename parts have to be directly after each other.
				// So reset if there is a hole.
				resetLongFilename(i)
				continue
			}

			longFilename = append(longFilename, longFilenameEntry)
			lastLongFilenameIndex = i
			continue
		}

		// Filter out not displayed entries.
		if entry.Attribute&AttrVolumeId == AttrVolumeId {
			continue
		}

		newEntry := ExtendedEntryHeader{EntryHeader: entry}
		// If the longFilename exists and the last longFilename part was the directly previous entry.
		if longFilename != nil && lastLongFilenameIndex+1 == i {
			// Calculate the checksum for the entry.
			var checksum byte = 0
			for i := 0; i < 11; i++ {
				checksum = (((checksum & 1) << 7) | ((checksum & 0xfe) >> 1)) + newEntry.Name[i]
			}

			var chars []uint16
			var valid = true

			// Run through the filename parts in reverse order.
			// Check the checksum and sequence numbers for each entry.
			// If everything is valid, save the full long name.
			sequenceNumber := 0
			for longFilenameIndex := len(longFilename) - 1; longFilenameIndex >= 0; longFilenameIndex-- {
				sequenceNumber++

				current := longFilename[longFilenameIndex]
				// If any checksum is wrong, the long filename is corrupt.
				if current.Checksum != checksum {
					valid = false
					break
				}

				// If any sequence number is invalid, the long filename is corrupt.
				// A correct long filename looks like this:
				//  <proceeding files...>
				//  <slot #3, id = 0x43, characters = "h is long">
				//  <slot #2, id = 0x02, characters = "xtension whic">
				//  <slot #1, id = 0x01, characters = "My Big File.E">
				//  <directory entry, name = "MYBIGFIL.EXT">
				// (the 0x40 bit is already checked above)
				if current.Sequence&0b0001111 != byte(sequenceNumber) {
					valid = false
					break
				}

				chars = append(chars, current.First[:]...)
				chars = append(chars, current.Second[:]...)
				chars = append(chars, current.Third[:]...)
			}

			if valid {
				for _, char := range chars {
					if char == 0 {
						break
					}
					// TODO: Not sure if fmt.Sprintf() in combination with rune() decodes the char correctly in all cases.
					// 		 Note for this:  Each Unicode character takes either two or four bytes, UTF-16LE encoded.
					newEntry.ExtendedName += fmt.Sprintf("%c", rune(char))
				}
			}
		}
		directory = append(directory, newEntry)

		// Reset long filename for next file.
		resetLongFilename(i)
	}

	return directory, nil
}

func (f *Fs) readDirAtSector(sectorNum uint32) ([]ExtendedEntryHeader, error) {
	rootDirSectorsCount := uint32(((f.info.RootEntryCount * 32) + (f.info.BytesPerSector - 1)) / f.info.BytesPerSector)

	data := make([]byte, 0)

	for i := uint32(0); i < rootDirSectorsCount; i++ {
		sector, err := f.fetch(sectorNum + i)
		if err != nil {
			return nil, checkpoint.Wrap(err, ErrReadFilesystemDir)
		}

		newData := make([]byte, f.info.BytesPerSector)
		err = binary.Read(bytes.NewReader(sector.buffer), binary.LittleEndian, &newData)
		if err != nil {
			return nil, checkpoint.Wrap(err, ErrReadFilesystemDir)
		}

		data = append(data, newData...)
	}

	return f.parseDir(data)
}

func (f *Fs) readDir(cluster fatEntry) ([]ExtendedEntryHeader, error) {
	data, err := f.readFileAt(cluster, -1, 0, 0)
	if err != nil {
		return nil, checkpoint.Wrap(err, ErrReadFilesystemDir)
	}

	return f.parseDir(data)
}

// readRoot either reads the root directory either from the specific root sector if the type is < FAT32 or
// from the first root cluster if the type is FAT32.
func (f *Fs) readRoot() ([]ExtendedEntryHeader, error) {
	if f.info.FSType == FAT12 {
		checkpoint.From(ErrNotSupported)
	}

	var root []ExtendedEntryHeader
	var err error
	switch f.info.FSType {
	case FAT16:
		firstRootSector := uint32(f.info.ReservedSectorCount) + (uint32(f.info.FatCount) * f.info.FatSize)
		root, err = f.readDirAtSector(firstRootSector)
	case FAT32:
		root, err = f.readDir(f.info.fat32Specific.RootCluster)
	}

	return root, checkpoint.Wrap(err, ErrReadFilesystemDir)
}

// initialize a FAT filesystem. Some checks are done to validate if it is a valid FAT filesystem.
// (If skipping checks is disabled.)
// It also calculates the filesystem type.
func (f *Fs) initialize(skipChecks bool) error {
	_, err := f.reader.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	// The data for the first sector is always in the first 512 so use that until the correct sector size is loaded.
	// Note that almost all FAT filesystems use 512.
	// Some may use 1024, 2048 or 4096 but this is not supported by many drivers.
	f.info.BytesPerSector = 512

	// Read sec0
	// Set to a sector unequal 0 to avoid using empty buffer in fetch.
	f.sectorCache.current = 0xFFFFFFFF
	sector, err := f.fetch(0)
	if err != nil {
		return err
	}

	// Read sector as BPB
	bpb := BPB{}
	err = binary.Read(bytes.NewReader(sector.buffer), binary.LittleEndian, &bpb)
	if err != nil {
		return checkpoint.Wrap(err, fmt.Errorf("%w: parsing the bpb sector failed", ErrInitializeFilesystem))
	}

	if !skipChecks {
		// Check if it is really a FAT filesystem.
		// Check for valid jump instructions
		if !(bpb.BSJumpBoot[0] == 0xEB && bpb.BSJumpBoot[2] == 0x90) && !(bpb.BSJumpBoot[0] == 0xE9) {
			return checkpoint.From(fmt.Errorf("%w: no valid jump instructions at the beginning", ErrInitializeFilesystem))
		}

		// Load the sector size and use it for all following sector reads.
		// Also FAT only supports 512, 1024, 2048 and 4096
		if bpb.BytesPerSector != 512 && bpb.BytesPerSector != 1024 && bpb.BytesPerSector != 2048 && bpb.BytesPerSector != 4096 {
			return checkpoint.From(fmt.Errorf("%w: invalid sector size", ErrInitializeFilesystem))
		}

		// Sectors per cluster has to be a power of two and greater than 0.
		// Also the whole cluster size should not be more than 32K.
		if bpb.SectorsPerCluster%2 != 0 || bpb.SectorsPerCluster == 0 || (bpb.BytesPerSector*uint16(bpb.SectorsPerCluster)) > (32*1024) {
			return checkpoint.From(fmt.Errorf("%w: invalid sectors per cluster", ErrInitializeFilesystem))
		}

		// The reserved sector count should not be 0.
		// Note: for FAT12 and FAT16 it is typically 1 for FAT32 it is typically 32.
		if bpb.ReservedSectorCount == 0 {
			return checkpoint.From(fmt.Errorf("%w: invalid reserved sector count", ErrInitializeFilesystem))
		}

		if bpb.NumFATs < 1 {
			return checkpoint.From(fmt.Errorf("%w: invalid FAT count", ErrInitializeFilesystem))
		}

		if bpb.Media != 0xF0 &&
			!(bpb.Media >= 0xF8 && bpb.Media <= 0xFF) {
			return checkpoint.From(fmt.Errorf("%w: invalid media value", ErrInitializeFilesystem))
		}

		if sector.buffer[510] != 0x55 || sector.buffer[511] != 0xAA {
			return checkpoint.From(fmt.Errorf("%w: invalid signature at offset 510 / 511", ErrInitializeFilesystem))
		}
	}

	var totalSectors, dataSectors, countOfClusters uint32

	// Calculate the cluster count to determine the FAT type.
	var rootDirSectors uint32 = ((uint32(bpb.RootEntryCount) * 32) + (uint32(bpb.BytesPerSector) - 1)) / uint32(bpb.BytesPerSector)

	if bpb.FATSize16 != 0 {
		f.info.FatSize = uint32(bpb.FATSize16)
	} else {
		// Read the FAT32 specific data.
		err = binary.Read(bytes.NewReader(bpb.FATSpecificData[:]), binary.LittleEndian, &f.info.fat32Specific)
		if err != nil {
			return checkpoint.Wrap(err, fmt.Errorf("%w: parsing the fat32 specific data failed", ErrInitializeFilesystem))
		}
		f.info.FatSize = f.info.fat32Specific.FatSize
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
		// For now do not support FAT12 as its a bit more complicated.
		return checkpoint.From(fmt.Errorf("%w: FAT12 is not supported", ErrNotSupported))
	} else if countOfClusters < 65525 {
		f.info.FSType = FAT16
	} else {
		f.info.FSType = FAT32
	}

	// The root entry count has to be 0 for FAT32 and has to fit exactly into the sectors.
	if f.info.FSType == FAT32 && bpb.RootEntryCount != 0 || (f.info.FSType != FAT32 && (bpb.RootEntryCount*32)%bpb.BytesPerSector != 0) {
		return checkpoint.From(fmt.Errorf("%w: invalid root entry count", ErrInitializeFilesystem))
	}

	// Now all needed data can be saved. See FAT spec for details.
	f.info.BytesPerSector = bpb.BytesPerSector
	if bpb.TotalSectors16 != 0 {
		f.info.TotalSectorCount = uint32(bpb.TotalSectors16)
	} else {
		f.info.TotalSectorCount = bpb.TotalSectors32
	}
	dataSectors = f.info.TotalSectorCount - (uint32(bpb.ReservedSectorCount) + (uint32(bpb.NumFATs) * f.info.FatSize) + rootDirSectors)
	f.info.SectorsPerCluster = bpb.SectorsPerCluster
	f.info.ReservedSectorCount = bpb.ReservedSectorCount
	f.info.FirstDataSector = uint32(bpb.ReservedSectorCount) + (uint32(bpb.NumFATs) * f.info.FatSize) + rootDirSectors
	f.info.FatCount = bpb.NumFATs
	f.info.RootEntryCount = bpb.RootEntryCount

	if f.info.FSType == FAT32 {
		f.info.Label = string(f.info.fat32Specific.BSVolumeLabel[:])
	} else {
		err = binary.Read(bytes.NewReader(bpb.FATSpecificData[:]), binary.LittleEndian, &f.info.fat16Specific)
		if err != nil {
			return checkpoint.Wrap(err, fmt.Errorf("%w: parsing the fat16 specific data failed", ErrInitializeFilesystem))
		}

		f.info.Label = string(f.info.fat16Specific.BSVolumeLabel[:])
	}

	return nil
}

// fetch loads a specific single sector of the filesystem.
func (f *Fs) fetch(sectorNum uint32) (Sector, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	sector := Sector{
		buffer: make([]byte, f.info.BytesPerSector),
	}

	// Only load it once.
	if sectorNum == f.sectorCache.current {
		return f.sectorCache, nil
	}

	// Seek to and Read the new sectorNum.
	_, err := f.reader.Seek(int64(sectorNum)*int64(f.info.BytesPerSector), io.SeekStart)
	if err != nil {
		return Sector{}, checkpoint.Wrap(err, fmt.Errorf("%w: sector %d", ErrFetchingSector, sectorNum))
	}

	_, err = f.reader.Read(sector.buffer)
	if err != nil {
		return Sector{}, checkpoint.Wrap(err, fmt.Errorf("%w: sector %d", ErrFetchingSector, sectorNum))
	}

	sector.current = sectorNum
	f.sectorCache = sector
	return sector, nil
}

type fatEntry uint32

func (e fatEntry) Value() uint32 {
	return uint32(e)
}

// IsFree only returns true if the sector is unused.
func (e fatEntry) IsFree() bool {
	return (e & 0x0FFFFFFF) == 0x00000000
}

// IsReservedTemp is a special value used to mark clusters as tmp-eof e.g. while writing data to it.
// It should be treated like EOF. Use ReadAsEOF to check for all EOF-like values.
func (e fatEntry) IsReservedTemp() bool {
	return (e & 0x0FFFFFFF) == 0x00000001
}

// IsNextCluster is true if the cluster is a normal data cluster.
// Use ReadAsNextCluster to check for all DataCluster-like values.
func (e fatEntry) IsNextCluster() bool {
	masked := e & 0x0FFFFFFF
	return masked >= 0x00000002 && masked <= 0x0FFFFFEF
}

// IsReservedSometimes is a special value which may occur in rare cases. Should be treated as a DataCluster.
// TODO: For FAT12 a special case exists -> 0xFF0 should be read as EOF. This is not implemented yet.
// Use ReadAsNextCluster to check for all DataCluster-like values.
func (e fatEntry) IsReservedSometimes() bool {
	masked := e & 0x0FFFFFFF
	return masked >= 0x0FFFFFF0 && masked <= 0x0FFFFFF5
}

// IsReserved is a special value which may occur in rare cases. Should be treated as a DataCluster.
// Use ReadAsNextCluster to check for all DataCluster-like values.
func (e fatEntry) IsReserved() bool {
	return (e & 0x0FFFFFFF) == 0x0FFFFFF6
}

// IsBad is a special value which indicates a bad sector. Should be treated as a DataCluster.
// Use ReadAsNextCluster to check for all DataCluster-like values.
func (e fatEntry) IsBad() bool {
	return (e & 0x0FFFFFFF) == 0x0FFFFFF7
}

// IsEOF is a special value used to mark clusters as EOF.
// Use ReadAsEOF to check for all EOF-like values.
func (e fatEntry) IsEOF() bool {
	masked := e & 0x0FFFFFFF
	return masked >= 0x0FFFFFF8 && masked <= 0x0FFFFFFF
}

// ReadAsNextCluster treats all values specified as "should be used as Data Cluster" in
// https://en.wikipedia.org/wiki/Design_of_the_FAT_file_system#Cluster_values
// as normal data clusters.
// Use this tho check if it should be read as a normal data cluster.
func (e fatEntry) ReadAsNextCluster() bool {
	// TODO: e.IsReservedSometimes(): MS-DOS/PC DOS 3.3 and higher treats a value of 0xFF0[nb 11][13] on FAT12 (but not on FAT16 or FAT32)
	//       volumes as additional end-of-chain marker similar to 0xFF8-0xFFF.[13] For compatibility with MS-DOS/PC DOS,
	//       file systems should avoid to use data cluster 0xFF0 in cluster chains on FAT12 volumes (that is, treat it
	//       as a reserved cluster similar to 0xFF7). (NB. The correspondence of the low byte of the cluster number with
	//       the FAT ID and media descriptor values is the reason, why these cluster values are reserved.)

	return e.IsNextCluster() || e.IsReservedSometimes() || e.IsReserved() || e.IsBad()
}

// ReadAsEOF treats all values specified as "should be read as EOF" in
// https://en.wikipedia.org/wiki/Design_of_the_FAT_file_system#Cluster_values
// as EOF.
// Use this to check if it should be read as an EOF.
func (e fatEntry) ReadAsEOF() bool {
	return e.IsEOF() || e.IsReservedTemp()
}

// getFatEntry returns the next fat entry for the given cluster.
func (f *Fs) getFatEntry(cluster fatEntry) (fatEntry, error) {
	if f.info.FSType == FAT12 {
		return 0, checkpoint.From(ErrNotSupported)
	}

	var fatOffset uint32
	switch f.info.FSType {
	case FAT16:
		fatOffset = cluster.Value() * 2
	case FAT32:
		fatOffset = cluster.Value() * 4
	}

	fatSectorNumber := uint32(f.info.ReservedSectorCount) + (fatOffset / uint32(f.info.BytesPerSector))
	fatEntryOffset := fatOffset % uint32(f.info.BytesPerSector)

	sector, err := f.fetch(fatSectorNumber)
	if err != nil {
		return 0, checkpoint.Wrap(err, ErrReadFat)
	}

	switch f.info.FSType {
	case FAT16:
		fat16ClusterEntryValue := binary.LittleEndian.Uint16(sector.buffer[fatEntryOffset : fatEntryOffset+2])

		// convert the special values to FAT32 special values (e.g. 0xFF -> 0x0FFFFFFF)
		if fat16ClusterEntryValue >= 0xFFF0 && fat16ClusterEntryValue <= 0xFFFF {
			return fatEntry(uint32(fat16ClusterEntryValue) | 0x0FFFF000&0x0FFFFFFF), nil
		}

		return fatEntry(fat16ClusterEntryValue), nil
	case FAT32:
		fat32ClusterEntryValue := binary.LittleEndian.Uint32(sector.buffer[fatEntryOffset:fatEntryOffset+4]) & 0x0FFFFFFF
		return fatEntry(fat32ClusterEntryValue), nil
	}

	return 0, checkpoint.From(ErrNotSupported)
}

func (f *Fs) store() error {
	panic("implement me")
}

func (f *Fs) Label() string {
	// TODO: There may be a label entry in the root folder. Check how that should be handled.
	return strings.TrimRight(f.info.Label, " ")
}

func (f *Fs) FSType() FATType {
	return f.info.FSType
}

func (f *Fs) Create(name string) (afero.File, error) {
	panic("implement me")
}

func (f *Fs) Mkdir(name string, perm os.FileMode) error {
	panic("implement me")
}

func (f *Fs) MkdirAll(path string, perm os.FileMode) error {
	panic("implement me")
}

func (f *Fs) Open(path string) (afero.File, error) {
	if !fs.ValidPath(path) {
		return nil, checkpoint.Wrap(ErrInvalidPath, ErrOpenFilesystem)
	}
	path = filepath.ToSlash(path)

	if path == "." {
		path = ""
	}

	// For root just return a fake-file.
	if path == "" {
		fakeEntry := ExtendedEntryHeader{
			EntryHeader: EntryHeader{
				Name:      [11]byte{' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' '},
				Attribute: AttrDirectory,
			},
		}

		return &File{
			fs:          f,
			path:        path,
			isDirectory: true,
			stat:        fakeEntry.FileInfo(),
		}, nil
	}

	// Remove suffix-slash.
	path = strings.TrimSuffix(path, "/")
	dirParts := strings.Split(path, "/")

	content, err := f.readRoot()
	if err != nil {
		return nil, checkpoint.Wrap(err, ErrOpenFilesystem)
	}

	// Go through the path until the last pathPart and then use the contents of that folder as result.
pathLoop:
	for i, pathPart := range dirParts {
		if pathPart == "" {
			continue
		}

		for _, entry := range content {
			fileInfo := entry.FileInfo()
			// Note: FAT is not case sensitive.
			if strings.ToUpper(strings.Trim(fileInfo.Name(), " ")) == strings.ToUpper(pathPart) {
				// If it is the last one return it as a File.
				if i == len(dirParts)-1 {
					return &File{
						fs:           f,
						path:         path,
						isDirectory:  fileInfo.IsDir(),
						isReadOnly:   entry.Attribute&AttrReadOnly == AttrReadOnly,
						isHidden:     entry.Attribute&AttrHidden == AttrHidden,
						isSystem:     entry.Attribute&AttrSystem == AttrSystem,
						firstCluster: fatEntry(uint32(entry.FirstClusterHI)<<16 | uint32(entry.FirstClusterLO)),
						stat:         entry.FileInfo(),
					}, nil
				}

				// Else try to go deeper.
				if !fileInfo.IsDir() {
					return nil, checkpoint.Wrap(syscall.ENOTDIR, ErrOpenFilesystem)
				}

				content, err = f.readDir(fatEntry(uint32(entry.FirstClusterHI)<<16 | uint32(entry.FirstClusterLO)))
				if err != nil {
					return nil, checkpoint.Wrap(err, ErrOpenFilesystem)
				}

				continue pathLoop
			}
		}
		return nil, checkpoint.Wrap(ErrOpenFilesystem, errors.New("no matching path found: ***/"+pathPart+"/***"))
	}

	return nil, checkpoint.Wrap(ErrOpenFilesystem, errors.New("path doesn't exist: "+path))
}

func (f *Fs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	// TODO: implement flag and perm
	return f.Open(name)
}

func (f *Fs) Remove(name string) error {
	panic("implement me")
}

func (f *Fs) RemoveAll(path string) error {
	panic("implement me")
}

func (f *Fs) Rename(oldname, newname string) error {
	panic("implement me")
}

func (f *Fs) Stat(path string) (os.FileInfo, error) {
	file, err := f.Open(path)
	if err != nil {
		return nil, checkpoint.From(errors.New("path doesn't exist: " + path))
	}
	defer func() {
		_ = file.Close()
	}()

	return file.Stat()
}

func (f *Fs) Name() string {
	return "FAT"
}

func (f *Fs) Chmod(name string, mode os.FileMode) error {
	panic("implement me")
}

func (f *Fs) Chown(name string, uid, gid int) error {
	panic("implement me")
}

func (f *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	panic("implement me")
}
