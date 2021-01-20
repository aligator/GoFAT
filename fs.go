package gofat

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	AttrArchive   = 0x12
	AttrLongName  = AttrReadOnly | AttrHidden | AttrSystem | AttrVolumeId
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
		return nil, err
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
		return nil, err
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
func (fs *Fs) readFileAt(cluster fatEntry, fileSize int64, offset int64, readSize int64) ([]byte, error) {
	// finalize returns the data sliced to either the readSize, the fileSize or 'as it is'.
	// It may return io.EOF if readSize + offset > fileSize.
	finalize := func(result []byte, err error) ([]byte, error) {
		if fileSize < 0 {
			fileSize = int64(len(result)) + offset
		}

		if err == nil && readSize > fileSize-offset {
			err = io.EOF
			readSize = fileSize - offset
		}

		// Return at most the readSize as requested.
		// A readSize of <= 0 means to return till EOF.
		if readSize > 0 && int64(len(result)) > readSize {
			return result[:readSize], err
		}

		// Return the whole file
		if int64(len(result)) > fileSize {
			return result[:fileSize], err
		}

		// Else just return the result.
		return result, err
	}

	data := make([]byte, 0)

	clusterNumber := 0
	currentCluster := cluster

	// offsetRest contains the offset which is needed for the actual first sector.
	// for more easy calculation. This value gets updated everytime some bytes get skipped.
	offsetRest := offset

	// Find the cluster to start.
	// We still have to load the cluster number chain.
	for {
		if int64(clusterNumber)*int64(fs.info.SectorsPerCluster)*int64(fs.info.BytesPerSector) <= offset &&
			int64(clusterNumber+1)*int64(fs.info.SectorsPerCluster)*int64(fs.info.BytesPerSector) >= offset {
			break
		}

		nextCluster, err := fs.getFatEntry(currentCluster)
		if err != nil {
			return finalize(data, err)
		}

		if !nextCluster.ReadAsNextCluster() {
			return finalize(data, nil)
		}

		currentCluster = nextCluster
		clusterNumber++
	}

	offsetRest -= int64(clusterNumber) * int64(fs.info.SectorsPerCluster) * int64(fs.info.BytesPerSector)

	skip := uint8(0)
	// Calculate the sectors to skip for the first sector.
	skip = uint8(offsetRest / int64(fs.info.BytesPerSector))

	// Calculate the offsetRest -> the amount of bytes to skip on the first read sector.
	offsetRest -= int64(fs.info.BytesPerSector) * int64(skip)

	// Read the clusters.
	for {
		firstSectorOfCluster := ((currentCluster.Value() - 2) * uint32(fs.info.SectorsPerCluster)) + fs.info.FirstDataSector

		// Read the sectors of the cluster
		for i := skip; i < fs.info.SectorsPerCluster; i++ {
			skip = 0
			sector, err := fs.fetch(firstSectorOfCluster + uint32(i))
			if err != nil {
				return finalize(data, err)
			}

			newData := make([]byte, fs.info.BytesPerSector)
			err = binary.Read(bytes.NewReader(sector.buffer), binary.LittleEndian, &newData)
			if err != nil {
				return finalize(nil, err)
			}

			// Trim the first bytes based on the offset if it is the first read.
			if len(data) == 0 {
				data = append(data, newData[offsetRest:]...)
				continue
			}

			data = append(data, newData...)
		}

		// Stop when the size needed is reached.
		if readSize > int64(0) && int64(clusterNumber+1)*int64(fs.info.SectorsPerCluster)*int64(fs.info.BytesPerSector) >= offset+readSize {
			break
		}

		nextCluster, err := fs.getFatEntry(currentCluster)
		if err != nil {
			return finalize(data, err)
		}

		if !nextCluster.ReadAsNextCluster() {
			break
		}

		currentCluster = nextCluster
		clusterNumber++
	}

	return finalize(data, nil)
}

// parseDir reads and interprets a directory-file. It returns a slice of ExtendedEntryHeader,
// one for each file in the directory. It may return an error if it cannot be parsed.
func (fs *Fs) parseDir(data []byte) ([]ExtendedEntryHeader, error) {
	entries := make([]EntryHeader, len(data)/32)

	err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &entries)
	if err != nil {
		return nil, err
	}

	// Convert to fatFiles and filter empty entries.
	var longFilename []LongFilenameEntry
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
				return nil, err
			}

			// Ignore deleted entry.
			if longFilenameEntry.Sequence == 0xE5 {
				continue
			}

			longFilename = append(longFilename, longFilenameEntry)
			continue
		}

		// Filter out not displayed entries.
		if entry.Attribute&AttrVolumeId == AttrVolumeId {
			// Reset long filename for next file.
			longFilename = nil
			continue
		}

		newEntry := ExtendedEntryHeader{EntryHeader: entry}
		if longFilename != nil {
			sort.SliceStable(longFilename, func(i, j int) bool {
				// Sort by the Sequence number field:
				// (bit 6: last logical, first physical LFN entry, bit 5: 0; bits 4-0: number 0x01..0x14 (0x1F), deleted entry: 0xE5)
				return longFilename[i].Sequence&0b0001111 < longFilename[j].Sequence&0b0001111
			})

			// TODO: check checksum and if invalid ignore the long name.
			//       Also add more sanity checks. e.g. the long filename entries have to occur in a continuous way
			//       without something in between.

			var chars []uint16
			for _, namePart := range longFilename {
				chars = append(chars, namePart.First[:]...)
				chars = append(chars, namePart.Second[:]...)
				chars = append(chars, namePart.Third[:]...)
			}

			for _, char := range chars {
				if char == 0 {
					break
				}
				// TODO: Not sure if fmt.Sprintf() in combination with rune() decodes the two-byte char correctly in all cases.
				newEntry.ExtendedName += fmt.Sprintf("%c", rune(char))
			}
		}
		directory = append(directory, newEntry)

		// Reset long filename for next file.
		longFilename = nil
	}

	return directory, nil
}

func (fs *Fs) readDirAtSector(sectorNum uint32) ([]ExtendedEntryHeader, error) {
	rootDirSectorsCount := uint32(((fs.info.RootEntryCount * 32) + (fs.info.BytesPerSector - 1)) / fs.info.BytesPerSector)

	data := make([]byte, 0)

	for i := uint32(0); i < rootDirSectorsCount; i++ {
		sector, err := fs.fetch(sectorNum + i)
		if err != nil {
			return nil, err
		}

		newData := make([]byte, fs.info.BytesPerSector)
		err = binary.Read(bytes.NewReader(sector.buffer), binary.LittleEndian, &newData)
		if err != nil {
			return nil, err
		}

		data = append(data, newData...)
	}

	return fs.parseDir(data)
}

func (fs *Fs) readDir(cluster fatEntry) ([]ExtendedEntryHeader, error) {
	data, err := fs.readFileAt(cluster, -1, 0, 0)
	if err != nil {
		return nil, err
	}

	return fs.parseDir(data)
}

// readRoot either reads the root directory either from the specific root sector if the type is < FAT32 or
// from the first root cluster if the type is FAT32.
func (fs *Fs) readRoot() ([]ExtendedEntryHeader, error) {
	if fs.info.FSType == FAT12 {
		panic("not supported")
	}

	var root []ExtendedEntryHeader
	var err error
	switch fs.info.FSType {
	case FAT16:
		firstRootSector := uint32(fs.info.ReservedSectorCount) + (uint32(fs.info.FatCount) * fs.info.FatSize)
		root, err = fs.readDirAtSector(firstRootSector)
	case FAT32:
		root, err = fs.readDir(fs.info.fat32Specific.RootCluster)
	}

	return root, err
}

// initialize a FAT filesystem. Some checks are done to validate if it is a valid FAT filesystem.
// (If skipping checks is disabled.)
// It also calculates the filesystem type.
func (fs *Fs) initialize(skipChecks bool) error {
	_, err := fs.reader.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	// The data for the first sector is always in the first 512 so use that until the correct sector size is loaded.
	// Note that almost all FAT filesystems use 512.
	// Some may use 1024, 2048 or 4096 but this is not supported by many drivers.
	fs.info.BytesPerSector = 512

	// Read sec0
	// Set to a sector unequal 0 to avoid using empty buffer in fetch.
	fs.sectorCache.current = 0xFFFFFFFF
	sector, err := fs.fetch(0)
	if err != nil {
		return err
	}

	// Read sector as BPB
	bpb := BPB{}
	err = binary.Read(bytes.NewReader(sector.buffer), binary.LittleEndian, &bpb)
	if err != nil {
		return err
	}

	if !skipChecks {
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

		if bpb.NumFATs < 1 {
			return errors.New("invalid FAT count")
		}

		if bpb.Media != 0xF0 &&
			!(bpb.Media >= 0xF8 && bpb.Media <= 0xFF) {
			return errors.New("invalid media value")
		}

		if sector.buffer[510] != 0x55 || sector.buffer[511] != 0xAA {
			return errors.New("invalid signature at offset 510 / 511")
		}
	}

	var totalSectors, dataSectors, countOfClusters uint32

	// Calculate the cluster count to determine the FAT type.
	var rootDirSectors uint32 = ((uint32(bpb.RootEntryCount) * 32) + (uint32(bpb.BytesPerSector) - 1)) / uint32(bpb.BytesPerSector)

	if bpb.FATSize16 != 0 {
		fs.info.FatSize = uint32(bpb.FATSize16)
	} else {
		// Read the FAT32 specific data.
		err = binary.Read(bytes.NewReader(bpb.FATSpecificData[:]), binary.LittleEndian, &fs.info.fat32Specific)
		if err != nil {
			return err
		}
		fs.info.FatSize = fs.info.fat32Specific.FatSize
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
		return errors.New("FAT12 is not supported")
	} else if countOfClusters < 65525 {
		fs.info.FSType = FAT16
	} else {
		fs.info.FSType = FAT32
	}

	// The root entry count has to be 0 for FAT32 and has to fit exactly into the sectors.
	if fs.info.FSType == FAT32 && bpb.RootEntryCount != 0 || (fs.info.FSType != FAT32 && (bpb.RootEntryCount*32)%bpb.BytesPerSector != 0) {
		return errors.New("invalid root entry count")
	}

	// Now all needed data can be saved. See FAT spec for details.
	fs.info.BytesPerSector = bpb.BytesPerSector
	if bpb.TotalSectors16 != 0 {
		fs.info.TotalSectorCount = uint32(bpb.TotalSectors16)
	} else {
		fs.info.TotalSectorCount = bpb.TotalSectors32
	}
	dataSectors = fs.info.TotalSectorCount - (uint32(bpb.ReservedSectorCount) + (uint32(bpb.NumFATs) * fs.info.FatSize) + rootDirSectors)
	fs.info.SectorsPerCluster = bpb.SectorsPerCluster
	fs.info.ReservedSectorCount = bpb.ReservedSectorCount
	fs.info.FirstDataSector = uint32(bpb.ReservedSectorCount) + (uint32(bpb.NumFATs) * fs.info.FatSize) + rootDirSectors
	fs.info.FatCount = bpb.NumFATs
	fs.info.RootEntryCount = bpb.RootEntryCount

	if fs.info.FSType == FAT32 {
		fs.info.Label = string(fs.info.fat32Specific.BSVolumeLabel[:])
	} else {
		err = binary.Read(bytes.NewReader(bpb.FATSpecificData[:]), binary.LittleEndian, &fs.info.fat16Specific)
		if err != nil {
			return err
		}

		fs.info.Label = string(fs.info.fat16Specific.BSVolumeLabel[:])
	}

	return nil
}

// fetch loads a specific single sector of the filesystem.
func (fs *Fs) fetch(sectorNum uint32) (Sector, error) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	sector := Sector{
		buffer: make([]byte, fs.info.BytesPerSector),
	}

	// Only load it once.
	if sectorNum == fs.sectorCache.current {
		return fs.sectorCache, nil
	}

	// Seek to and Read the new sectorNum.
	_, err := fs.reader.Seek(int64(sectorNum)*int64(fs.info.BytesPerSector), io.SeekStart)
	if err != nil {
		return Sector{}, err
	}

	_, err = fs.reader.Read(sector.buffer)
	if err != nil {
		return Sector{}, err
	}

	sector.current = sectorNum
	fs.sectorCache = sector
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
func (fs *Fs) getFatEntry(cluster fatEntry) (fatEntry, error) {
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

	sector, err := fs.fetch(fatSectorNumber)
	if err != nil {
		return 0, err
	}

	switch fs.info.FSType {
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

	panic("not supported")
}

func (fs *Fs) store() error {
	panic("implement me")
}

func (fs *Fs) Label() string {
	// TODO: There may be a label entry in the root folder. Check how that should be handled.
	return strings.TrimRight(fs.info.Label, " ")
}

func (fs *Fs) FSType() FATType {
	return fs.info.FSType
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

func (fs *Fs) Open(path string) (afero.File, error) {
	path = filepath.ToSlash(path)

	if path == "" {
		path = "/"
	}

	// For root just return a fake-file.
	if path == "/" {
		fakeEntry := ExtendedEntryHeader{
			EntryHeader: EntryHeader{
				Name:      [11]byte{' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' '},
				Attribute: AttrDirectory,
			},
		}

		return &File{
			fs:          fs,
			path:        path,
			isDirectory: true,
			stat:        fakeEntry.FileInfo(),
		}, nil
	}

	// Remove suffix-slash.
	path = strings.TrimSuffix(path, "/")
	dirParts := strings.Split(path, "/")

	content, err := fs.readRoot()
	if err != nil {
		return nil, err
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
						fs:           fs,
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
					return nil, syscall.ENOTDIR
				}

				content, err = fs.readDir(fatEntry(uint32(entry.FirstClusterHI)<<16 | uint32(entry.FirstClusterLO)))
				if err != nil {
					return nil, err
				}

				continue pathLoop
			}
		}
		return nil, errors.New("no matching path found: ***/" + pathPart + "/***")
	}

	return nil, errors.New("path doesn't exist: " + path)
}

func (fs *Fs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	// TODO: implement flag and perm
	return fs.Open(name)
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

func (fs *Fs) Stat(path string) (os.FileInfo, error) {
	file, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Stat()
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
