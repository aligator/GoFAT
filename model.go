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
	BSBootSig        byte
	BSVolumeId       uint32
	BSVolumeLabel    [11]byte
	BSFileSystemType [8]byte
}

type FAT32SpecificData struct {
	FatSize          uint32
	ExtFlags         uint16
	FSVersion        uint16
	RootCluster      uint32
	FSInfo           uint16
	BkBootSector     uint16
	Reserved         [12]byte
	BSDriveNumber    byte
	BSReserved1      byte
	BSBootSig        byte
	BSVolumeID       uint32
	BSVolumeLabel    [11]byte
	BSFileSystemType [8]byte
}

type Directory struct {
	Name         [11]byte
	Attr         byte
	NTRes        byte
	CrtTimeTenth byte
	CrtTime      uint16
	CrtDate      uint16
	LstAccDate   uint16
	FstClusHI    uint16
	WrtTime      uint16
	WrtDate      uint16
	FstClusLO    uint16
	FileSize     uint32
}
