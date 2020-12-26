// File model contains the structs which match the direct structures of the FAT filesystem.

package gofat

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

type FAT16SpecificData struct {
	BSDriveNumber    byte
	BSReserved1      byte
	BSBootSignature  byte
	BSVolumeId       uint32
	BSVolumeLabel    [11]byte
	BSFileSystemType [8]byte
}

type FAT32SpecificData struct {
	FatSize          uint32
	ExtFlags         uint16
	FSVersion        uint16
	RootCluster      fatEntry
	FSInfo           uint16
	BkBootSector     uint16
	Reserved         [12]byte
	BSDriveNumber    byte
	BSReserved1      byte
	BSBootSignature  byte
	BSVolumeID       uint32
	BSVolumeLabel    [11]byte
	BSFileSystemType [8]byte
}

type EntryHeader struct {
	Name            [11]byte
	Attribute       byte
	NTReserved      byte
	CreateTimeTenth byte
	CreateTime      uint16
	CreateDate      uint16
	LastAccessDate  uint16
	FirstClusterHI  uint16
	WriteTime       uint16
	WriteDate       uint16
	FirstClusterLO  uint16
	FileSize        uint32
}

type LongFilenameEntry struct {
	Sequence  byte
	First     [5]uint16
	Attribute byte
	EntryType byte
	Checksum  byte
	Second    [6]uint16
	Zero      [2]byte
	Third     [2]uint16
}

type ExtendedEntryHeader struct {
	EntryHeader
	ExtendedName string
}
