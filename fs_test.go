package gofat

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"github.com/spf13/afero"
)

const testFolderInImages = "DoNotEdit_tests"

const (
	fat32                         = "./testdata/fat32.img"
	fat16                         = "./testdata/fat16.img"
	fat32InvalidSectorsPerCluster = "./testdata/fat32-invalid-sectors-per-cluster.img"
	fat16InvalidFiles             = "./testdata/fat16-invalid-files.img"
)

func testFileReader(file string) io.ReadSeeker {
	fsFile, err := os.Open(file)
	if err != nil {
		fmt.Println("Make sure you ran go generate.")
		panic(err)
	}

	return fsFile
}

func testingNew(t testing.TB, reader io.ReadSeeker) *Fs {
	fs, err := New(reader)
	if err != nil {
		t.Error(err)
	}

	return fs
}

func TestNew(t *testing.T) {
	type args struct {
		reader io.ReadSeeker
	}
	tests := []struct {
		name string
		args args
		// Do not expect something special. Should be enough to check for non-nil.
		// Would not be that easy to provide a valid Fs to check with DeepEqual.
		wantNotNil bool
		wantErr    bool
	}{
		{
			name: "FAT32 test image",
			args: args{
				reader: testFileReader(fat32),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "FAT16 test image",
			args: args{
				reader: testFileReader(fat16),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "no FAT file",
			args: args{
				reader: strings.NewReader("This is no FAT file"),
			},
			wantNotNil: false,
			wantErr:    true,
		},
		{
			name: "fat32 invalid sectors per cluster test image",
			args: args{
				reader: testFileReader(fat32InvalidSectorsPerCluster),
			},
			wantNotNil: false,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (got != nil) != tt.wantNotNil {
				t.Errorf("New() = %v, wantNotNil %v", got, tt.wantNotNil)
			}
		})
	}
}

func TestNewSkipChecks(t *testing.T) {
	type args struct {
		reader io.ReadSeeker
	}
	tests := []struct {
		name       string
		args       args
		wantNotNil bool
		wantErr    bool
	}{
		{
			name: "FAT32 test image",
			args: args{
				reader: testFileReader(fat32),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "FAT16 test image",
			args: args{
				reader: testFileReader(fat16),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "no FAT file",
			args: args{
				reader: strings.NewReader("This is no FAT file"),
			},
			wantNotNil: false,
			wantErr:    true,
		},
		{
			name: "fat32 invalid sectors per cluster test image",
			args: args{
				reader: testFileReader(fat32InvalidSectorsPerCluster),
			},
			wantNotNil: true,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSkipChecks(tt.args.reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSkipChecks() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (got != nil) != tt.wantNotNil {
				t.Errorf("New() = %v, wantNotNil %v", got, tt.wantNotNil)
			}
		})
	}
}

func Test_fatEntry_Value(t *testing.T) {
	tests := []struct {
		name string
		e    fatEntry
		want uint32
	}{
		{
			name: "a value",
			e:    42,
			want: 42,
		},
		{
			name: "zero",
			e:    0,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.Value(); got != tt.want {
				t.Errorf("fatEntry.Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

// fatEntryTests contains the data for a fatEntry test case.
type fatEntryTests struct {
	name  string
	eFrom fatEntry
	eTo   fatEntry
	want  bool
}

// testFatEntry executes the fatEntryTests using the given tests and execution method.
// It utilizes testing/quick to fuzz values in between of eFrom and eTo. The edge-values are always tested.
func testFatEntry(t *testing.T, tests []fatEntryTests, method string, execute func(e fatEntry) bool) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First test the edge cases.
			if got := execute(tt.eFrom); got != tt.want {
				t.Errorf("fatEntry(0x%x).%v() = %v, want %v", tt.eFrom, method, got, tt.want)
			}
			if got := execute(tt.eTo); got != tt.want {
				t.Errorf("fatEntry(0x%x).%v() = %v, want %v", tt.eTo, method, got, tt.want)
			}
		})

		// If the values are too near, random tests make no sense.
		if tt.eFrom == tt.eTo || tt.eTo-tt.eFrom <= 2 {
			return
		}

		t.Run("Random: "+tt.name, func(t *testing.T) {
			// Then test some values in between using random values.

			// maxCount is either eTo - eFrom or 10 if too big.
			// That way only max 100 values get tested.
			// This applies only to the default value of "-quickchecks".
			// If a higher value than "-quickchecks 100" is used,
			// more values than maxCount may be tested.
			maxCount := int(tt.eTo - tt.eFrom)
			if maxCount > 100 {
				maxCount = 100
			}

			// Utilize quick.Check to randomize the test.
			if err := quick.Check(func(entry fatEntry) bool {
				return tt.want == execute(entry)
			}, &quick.Config{
				MaxCountScale: float64(maxCount) / 100,
				Values: func(values []reflect.Value, rand *rand.Rand) {
					// Generate a random fatEntry in the current value range.
					var min = int(tt.eFrom + 1)
					var max = int(tt.eTo)
					for i := range values {
						values[i] = reflect.ValueOf(fatEntry(uint32(rand.Intn(max-min) + min)))
					}
				},
			}); err != nil {
				t.Errorf("fatEntry(RANDOM_VALUE).%v() failed:\n%v", method, err)
			}
		})
	}
}

func Test_fatEntry_IsFree(t *testing.T) {
	testFatEntry(t, []fatEntryTests{
		{
			name:  "IsFree",
			eFrom: 0x00000000,
			eTo:   0x00000000,
			want:  true,
		},
		{
			name:  "not IsFree",
			eFrom: 0x00000001,
			eTo:   0x0FFFFFFF,
			want:  false,
		},
		{
			name:  "IsFree with most significant byte set (special bits which should be ignored)",
			eFrom: 0xF0000000,
			eTo:   0xF0000000,
			want:  true,
		},
	}, "IsFree", func(e fatEntry) bool {
		return e.IsFree()
	})
}

func Test_fatEntry_IsReservedTemp(t *testing.T) {
	testFatEntry(t, []fatEntryTests{
		{
			name:  "IsReservedTemp",
			eFrom: 0x00000001,
			eTo:   0x00000001,
			want:  true,
		},
		{
			name:  "higher than IsReservedTemp",
			eFrom: 0x00000002,
			eTo:   0x0FFFFFFF,
			want:  false,
		},
		{
			name:  "lower than IsReservedTemp",
			eFrom: 0x00000000,
			eTo:   0x00000000,
			want:  false,
		},
		{
			name:  "IsReservedTemp with most significant byte set (special bits which should be ignored)",
			eFrom: 0xF0000001,
			eTo:   0xF0000001,
			want:  true,
		},
	}, "IsReservedTemp", func(e fatEntry) bool {
		return e.IsReservedTemp()
	})
}

func Test_fatEntry_IsNextCluster(t *testing.T) {
	testFatEntry(t, []fatEntryTests{
		{
			name:  "IsNextCluster",
			eFrom: 0x00000002,
			eTo:   0x0FFFFFEF,
			want:  true,
		},
		{
			name:  "higher than IsNextCluster",
			eFrom: 0x0FFFFFF0,
			eTo:   0x0FFFFFFF,
			want:  false,
		},
		{
			name:  "lower than IsNextCluster",
			eFrom: 0x00000000,
			eTo:   0x00000001,
			want:  false,
		},
		{
			name:  "IsNextCluster with most significant byte set (special bits which should be ignored)",
			eFrom: 0xF0000002,
			eTo:   0xF0000002,
			want:  true,
		},
	}, "IsNextCluster", func(e fatEntry) bool {
		return e.IsNextCluster()
	})
}

func Test_fatEntry_IsReservedSometimes(t *testing.T) {
	testFatEntry(t, []fatEntryTests{
		{
			name:  "IsReservedSometimes",
			eFrom: 0x0FFFFFF0,
			eTo:   0x0FFFFFF5,
			want:  true,
		},
		{
			name:  "higher than IsReservedSometimes",
			eFrom: 0x0FFFFFF6,
			eTo:   0x0FFFFFFF,
			want:  false,
		},
		{
			name:  "lower than IsReservedSometimes",
			eFrom: 0x00000000,
			eTo:   0x0FFFFFEF,
			want:  false,
		},
		{
			name:  "IsReservedSometimes with most significant byte set (special bits which should be ignored)",
			eFrom: 0xFFFFFFF0,
			eTo:   0xFFFFFFF0,
			want:  true,
		},
	}, "IsReservedSometimes", func(e fatEntry) bool {
		return e.IsReservedSometimes()
	})
}

func Test_fatEntry_IsReserved(t *testing.T) {
	testFatEntry(t, []fatEntryTests{
		{
			name:  "IsReserved",
			eFrom: 0x0FFFFFF6,
			eTo:   0x0FFFFFF6,
			want:  true,
		},
		{
			name:  "higher than IsReserved",
			eFrom: 0x0FFFFFF7,
			eTo:   0x0FFFFFFF,
			want:  false,
		},
		{
			name:  "lower than IsReserved",
			eFrom: 0x00000000,
			eTo:   0x0FFFFFF5,
			want:  false,
		},
		{
			name:  "IsReserved with most significant byte set (special bits which should be ignored)",
			eFrom: 0xFFFFFFF6,
			eTo:   0xFFFFFFF6,
			want:  true,
		},
	}, "IsReserved", func(e fatEntry) bool {
		return e.IsReserved()
	})
}

func Test_fatEntry_IsBad(t *testing.T) {
	testFatEntry(t, []fatEntryTests{
		{
			name:  "IsBad",
			eFrom: 0x0FFFFFF7,
			eTo:   0x0FFFFFF7,
			want:  true,
		},
		{
			name:  "higher than IsBad",
			eFrom: 0x0FFFFFF8,
			eTo:   0x0FFFFFFF,
			want:  false,
		},
		{
			name:  "lower than IsBad",
			eFrom: 0x00000000,
			eTo:   0x0FFFFFF6,
			want:  false,
		},
		{
			name:  "IsBad with most significant byte set (special bits which should be ignored)",
			eFrom: 0xFFFFFFF7,
			eTo:   0xFFFFFFF7,
			want:  true,
		},
	}, "IsBad", func(e fatEntry) bool {
		return e.IsBad()
	})
}

func Test_fatEntry_IsEOF(t *testing.T) {
	testFatEntry(t, []fatEntryTests{
		{
			name:  "IsEOF",
			eFrom: 0x0FFFFFF8,
			eTo:   0x0FFFFFFF,
			want:  true,
		},
		{
			name:  "lower than IsEOF",
			eFrom: 0x00000000,
			eTo:   0x0FFFFFF7,
			want:  false,
		},
		{
			name:  "IsEOF with most significant byte set (special bits which should be ignored)",
			eFrom: 0xFFFFFFF8,
			eTo:   0xFFFFFFF8,
			want:  true,
		},
	}, "IsEOF", func(e fatEntry) bool {
		return e.IsEOF()
	})
}

func Test_fatEntry_ReadAsNextCluster(t *testing.T) {
	testFatEntry(t, []fatEntryTests{
		{
			name:  "IsFree",
			eFrom: 0x00000000,
			eTo:   0x00000000,
			want:  false,
		},
		{
			name:  "IsReservedTemp",
			eFrom: 0x00000001,
			eTo:   0x00000001,
			want:  false,
		},
		{
			name:  "IsNextCluster",
			eFrom: 0x00000002,
			eTo:   0x0FFFFFEF,
			want:  true,
		},
		{
			name:  "IsReservedSometimes",
			eFrom: 0x0FFFFFF0,
			eTo:   0x0FFFFFF5,
			want:  true,
		},
		{
			name:  "IsReserved",
			eFrom: 0x0FFFFFF6,
			eTo:   0x0FFFFFF6,
			want:  true,
		},
		{
			name:  "IsBad",
			eFrom: 0x0FFFFFF7,
			eTo:   0x0FFFFFF7,
			want:  true,
		},
		{
			name:  "IsEOF",
			eFrom: 0x0FFFFFF8,
			eTo:   0x0FFFFFFF,
			want:  false,
		},
	}, "ReadAsNextCluster", func(e fatEntry) bool {
		return e.ReadAsNextCluster()
	})
}

func Test_fatEntry_ReadAsEOF(t *testing.T) {
	testFatEntry(t, []fatEntryTests{
		{
			name:  "IsFree",
			eFrom: 0x00000000,
			eTo:   0x00000000,
			want:  false,
		},
		{
			name:  "IsReservedTemp",
			eFrom: 0x00000001,
			eTo:   0x00000001,
			want:  true,
		},
		{
			name:  "IsNextCluster",
			eFrom: 0x00000002,
			eTo:   0x0FFFFFEF,
			want:  false,
		},
		{
			name:  "IsReservedSometimes",
			eFrom: 0x0FFFFFF0,
			eTo:   0x0FFFFFF5,
			want:  false,
		},
		{
			name:  "IsReserved",
			eFrom: 0x0FFFFFF6,
			eTo:   0x0FFFFFF6,
			want:  false,
		},
		{
			name:  "IsBad",
			eFrom: 0x0FFFFFF7,
			eTo:   0x0FFFFFF7,
			want:  false,
		},
		{
			name:  "IsEOF",
			eFrom: 0x0FFFFFF8,
			eTo:   0x0FFFFFFF,
			want:  true,
		},
	}, "ReadAsEOF", func(e fatEntry) bool {
		return e.ReadAsEOF()
	})
}

func TestFs_Label(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "a simple label",
			fields: fields{
				info: Info{
					Label: "A super label",
				},
			},
			want: "A super label",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if got := fs.Label(); got != tt.want {
				t.Errorf("Fs.Label() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFs_FSType(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	tests := []struct {
		name   string
		fields fields
		want   FATType
	}{
		{
			name: "FAT12",
			fields: fields{
				info: Info{
					FSType: FAT12,
				},
			},
			want: FAT12,
		},
		{
			name: "FAT16",
			fields: fields{
				info: Info{
					FSType: FAT16,
				},
			},
			want: FAT16,
		},
		{
			name: "FAT32",
			fields: fields{
				info: Info{
					FSType: FAT32,
				},
			},
			want: FAT32,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if got := fs.FSType(); got != tt.want {
				t.Errorf("Fs.FSType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFs_Create(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    afero.File
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			got, err := fs.Create(tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fs.Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Fs.Create() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFs_Mkdir(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		name string
		perm os.FileMode
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if err := fs.Mkdir(tt.args.name, tt.args.perm); (err != nil) != tt.wantErr {
				t.Errorf("Fs.Mkdir() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFs_MkdirAll(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		path string
		perm os.FileMode
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if err := fs.MkdirAll(tt.args.path, tt.args.perm); (err != nil) != tt.wantErr {
				t.Errorf("Fs.MkdirAll() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFs_Open(t *testing.T) {
	fakeRootEntry := ExtendedEntryHeader{
		EntryHeader: EntryHeader{
			Name:      [11]byte{' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' '},
			Attribute: AttrDirectory,
		},
	}
	fakeRootFile := File{
		path:        "",
		isDirectory: true,
		stat:        fakeRootEntry.FileInfo(),
	}

	fakeFolderEntry := ExtendedEntryHeader{
		ExtendedName: "DoNotEdit_tests",
		EntryHeader: EntryHeader{
			Name:            [11]byte{68, 79, 78, 79, 84, 69, 126, 49, 32, 32, 32},
			Attribute:       AttrDirectory,
			NTReserved:      0,
			CreateTimeTenth: 122,
			CreateTime:      39428,
			CreateDate:      21043,
			LastAccessDate:  21034,
			FirstClusterHI:  0,
			WriteTime:       39428,
			WriteDate:       21043,
			FirstClusterLO:  52,
			FileSize:        0,
		},
	}
	fakeFolderFile := File{
		fs:           nil,
		path:         "DoNotEdit_tests",
		isDirectory:  true,
		isReadOnly:   false,
		isHidden:     false,
		isSystem:     false,
		firstCluster: 52,
		stat:         fakeFolderEntry.FileInfo(),
		offset:       0,
	}

	fakeFileEntry := ExtendedEntryHeader{
		ExtendedName: "README.md",
		EntryHeader: EntryHeader{
			Name:            [11]byte{82, 69, 65, 68, 77, 69, 32, 32, 77, 68, 32},
			Attribute:       AttrArchive,
			NTReserved:      0,
			CreateTimeTenth: 122,
			CreateTime:      39428,
			CreateDate:      21043,
			LastAccessDate:  21043,
			FirstClusterHI:  0,
			WriteTime:       41936,
			WriteDate:       20890,
			FirstClusterLO:  53,
			FileSize:        10513,
		},
	}
	fakeFile := File{
		fs:           nil,
		path:         "DoNotEdit_tests/README.md",
		isDirectory:  false,
		isReadOnly:   false,
		isHidden:     false,
		isSystem:     false,
		firstCluster: 53,
		stat:         fakeFileEntry.FileInfo(),
		offset:       0,
	}

	fakeFolderEntryFAT16 := ExtendedEntryHeader{
		ExtendedName: "DoNotEdit_tests",
		EntryHeader: EntryHeader{
			Name:            [11]byte{68, 79, 78, 79, 84, 69, 126, 49, 32, 32, 32},
			Attribute:       AttrDirectory,
			NTReserved:      0,
			CreateTimeTenth: 82,
			CreateTime:      44897,
			CreateDate:      21044,
			LastAccessDate:  21044,
			FirstClusterHI:  0,
			WriteTime:       44897,
			WriteDate:       21044,
			FirstClusterLO:  4,
			FileSize:        0,
		},
	}
	fakeFolderFileFAT16 := File{
		fs:           nil,
		path:         "DoNotEdit_tests",
		isDirectory:  true,
		isReadOnly:   false,
		isHidden:     false,
		isSystem:     false,
		firstCluster: 4,
		stat:         fakeFolderEntryFAT16.FileInfo(),
		offset:       0,
	}

	fakeFileEntryFAT16 := ExtendedEntryHeader{
		ExtendedName: "README.md",
		EntryHeader: EntryHeader{
			Name:            [11]byte{82, 69, 65, 68, 77, 69, 32, 32, 77, 68, 32},
			Attribute:       AttrArchive,
			NTReserved:      0,
			CreateTimeTenth: 82,
			CreateTime:      44897,
			CreateDate:      21044,
			LastAccessDate:  21069,
			FirstClusterHI:  0,
			WriteTime:       41936,
			WriteDate:       20890,
			FirstClusterLO:  6,
			FileSize:        10513,
		},
	}
	fakeFileFAT16 := File{
		fs:           nil,
		path:         "DoNotEdit_tests/README.md",
		isDirectory:  false,
		isReadOnly:   false,
		isHidden:     false,
		isSystem:     false,
		firstCluster: 6,
		stat:         fakeFileEntryFAT16.FileInfo(),
		offset:       0,
	}

	type args struct {
		path string
	}
	tests := []struct {
		name    string
		fs      *Fs
		args    args
		want    *File
		wantErr bool
	}{
		{
			name: "root with '.'",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				path: ".",
			},
			want:    &fakeRootFile,
			wantErr: false,
		},
		{
			name: "'/'",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				path: "/",
			},
			wantErr: true,
		},
		{
			name: "folder",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				path: testFolderInImages,
			},
			want:    &fakeFolderFile,
			wantErr: false,
		},
		{
			name: "not existing folder",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				path: "/non-existing-folder",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "file",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				path: testFolderInImages + "/README.md",
			},
			want:    &fakeFile,
			wantErr: false,
		},
		{
			name: "FAT16 root with '.'",
			fs:   testingNew(t, testFileReader(fat16)),
			args: args{
				path: ".",
			},
			want:    &fakeRootFile,
			wantErr: false,
		},
		{
			name: "FAT16 '/'",
			fs:   testingNew(t, testFileReader(fat16)),
			args: args{
				path: "/",
			},
			wantErr: true,
		},
		{
			name: "FAT16 folder",
			fs:   testingNew(t, testFileReader(fat16)),
			args: args{
				path: testFolderInImages,
			},
			want:    &fakeFolderFileFAT16,
			wantErr: false,
		},
		{
			name: "FAT16 not existing folder",
			fs:   testingNew(t, testFileReader(fat16)),
			args: args{
				path: "/non-existing-folder",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "FAT16 file",
			fs:   testingNew(t, testFileReader(fat16)),
			args: args{
				path: testFolderInImages + "/README.md",
			},
			want:    &fakeFileFAT16,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the fs.
			if tt.want != nil {
				tt.want.fs = tt.fs
			}

			got, err := tt.fs.Open(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fs.Open() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.want == nil && got == nil {
				// Deep equal seems to not work with nil here.
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Fs.Open() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFs_OpenFile(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		name string
		flag int
		perm os.FileMode
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    afero.File
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			got, err := fs.OpenFile(tt.args.name, tt.args.flag, tt.args.perm)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fs.OpenFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Fs.OpenFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFs_Remove(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if err := fs.Remove(tt.args.name); (err != nil) != tt.wantErr {
				t.Errorf("Fs.Remove() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFs_RemoveAll(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if err := fs.RemoveAll(tt.args.path); (err != nil) != tt.wantErr {
				t.Errorf("Fs.RemoveAll() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFs_Rename(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		oldname string
		newname string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if err := fs.Rename(tt.args.oldname, tt.args.newname); (err != nil) != tt.wantErr {
				t.Errorf("Fs.Rename() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFs_Stat(t *testing.T) {
	// Not needed for now as it just opens a file and cals Stat on it.
	// So it's mostly tested already.
}

func TestFs_Name(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "has to be 'FAT'",
			want: "FAT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{}
			if got := fs.Name(); got != tt.want {
				t.Errorf("Fs.Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFs_Chmod(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		name string
		mode os.FileMode
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if err := fs.Chmod(tt.args.name, tt.args.mode); (err != nil) != tt.wantErr {
				t.Errorf("Fs.Chmod() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFs_Chown(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		name string
		uid  int
		gid  int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if err := fs.Chown(tt.args.name, tt.args.uid, tt.args.gid); (err != nil) != tt.wantErr {
				t.Errorf("Fs.Chown() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFs_Chtimes(t *testing.T) {
	type fields struct {
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	type args struct {
		name  string
		atime time.Time
		mtime time.Time
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        sync.Mutex{},
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if err := fs.Chtimes(tt.args.name, tt.args.atime, tt.args.mtime); (err != nil) != tt.wantErr {
				t.Errorf("Fs.Chtimes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFs_readFile(t *testing.T) {
	type args struct {
		cluster  fatEntry
		fileSize int64
		offset   int64
		readSize int64
	}
	tests := []struct {
		name    string
		fs      *Fs
		args    args
		want    []byte
		wantErr error
	}{
		{
			name: "read a file (without fileSize)",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				cluster:  53,
				fileSize: -1,
				offset:   0,
				readSize: 0,
			},
			want:    append([]byte("## GoFAT\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nAhis is an example FAT32 volume used to test GoFAT.\nBhis is an example FAT32 volume used to test GoFAT.\nChis is an example FAT32 volume used to test GoFAT.\nDhis is an example FAT32 volume used to test GoFAT.\nEhis is an example FAT32 volume used to test GoFAT.\nFhis is an example FAT32 volume used to test GoFAT.\nGhis is an example FAT32 volume used to test GoFAT.\nHhis is an example FAT32 volume used to test GoFAT.\nIhis is an example FAT32 volume used to test GoFAT.\nJhis is an example FAT32 volume used to test GoFAT.\nKhis is an example FAT32 volume used to test GoFAT.\nLhis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nAhis is an example FAT32 volume used to test GoFAT.\nBhis is an example FAT32 volume used to test GoFAT.\nChis is an example FAT32 volume used to test GoFAT.\nDhis is an example FAT32 volume used to test GoFAT.\nEhis is an example FAT32 volume used to test GoFAT.\nFhis is an example FAT32 volume used to test GoFAT.\nGhis is an example FAT32 volume used to test GoFAT.\nHhis is an example FAT32 volume used to test GoFAT.\nIhis is an example FAT32 volume used to test GoFAT.\nJhis is an example FAT32 volume used to test GoFAT.\nKhis is an example FAT32 volume used to test GoFAT.\nLhis is an example FAT32 volume used to test GoFAT.\n"), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
			wantErr: nil,
		},
		{
			name: "read the whole file (with file size)",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				cluster:  53,
				fileSize: 10513,
				offset:   0,
				readSize: 0,
			},
			want:    []byte("## GoFAT\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nAhis is an example FAT32 volume used to test GoFAT.\nBhis is an example FAT32 volume used to test GoFAT.\nChis is an example FAT32 volume used to test GoFAT.\nDhis is an example FAT32 volume used to test GoFAT.\nEhis is an example FAT32 volume used to test GoFAT.\nFhis is an example FAT32 volume used to test GoFAT.\nGhis is an example FAT32 volume used to test GoFAT.\nHhis is an example FAT32 volume used to test GoFAT.\nIhis is an example FAT32 volume used to test GoFAT.\nJhis is an example FAT32 volume used to test GoFAT.\nKhis is an example FAT32 volume used to test GoFAT.\nLhis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nAhis is an example FAT32 volume used to test GoFAT.\nBhis is an example FAT32 volume used to test GoFAT.\nChis is an example FAT32 volume used to test GoFAT.\nDhis is an example FAT32 volume used to test GoFAT.\nEhis is an example FAT32 volume used to test GoFAT.\nFhis is an example FAT32 volume used to test GoFAT.\nGhis is an example FAT32 volume used to test GoFAT.\nHhis is an example FAT32 volume used to test GoFAT.\nIhis is an example FAT32 volume used to test GoFAT.\nJhis is an example FAT32 volume used to test GoFAT.\nKhis is an example FAT32 volume used to test GoFAT.\nLhis is an example FAT32 volume used to test GoFAT.\n"),
			wantErr: nil,
		},
		{
			name: "read subset of a file",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				cluster:  53,
				fileSize: -1,
				offset:   0,
				readSize: 20,
			},
			want:    []byte("## GoFAT\nThis is an "),
			wantErr: nil,
		},
		{
			name: "read subset in the middle of a file",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				cluster:  53,
				fileSize: -1,
				offset:   10357,
				readSize: 52,
			},
			want:    []byte("Jhis is an example FAT32 volume used to test GoFAT.\n"),
			wantErr: nil,
		},
		{
			name: "read over EOF over last sector size (fileSize -1)",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				cluster:  53,
				fileSize: -1,
				offset:   10500,
				readSize: 9999,
			},
			want:    append([]byte(" test GoFAT.\n"), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
			wantErr: io.EOF,
		},
		{
			name: "read over EOF with file size",
			fs:   testingNew(t, testFileReader(fat32)),
			args: args{
				cluster:  53,
				fileSize: 10513,
				offset:   10500,
				readSize: 15,
			},
			want:    []byte(" test GoFAT.\n"),
			wantErr: io.EOF,
		},
		{
			name: "seek over cluster bound with wrong cluster marker (EOC)",
			fs:   testingNew(t, testFileReader(fat16InvalidFiles)),
			args: args{
				cluster:  6,
				fileSize: 10513,
				offset:   10500,
				readSize: 15,
			},
			want:    []byte{},
			wantErr: io.EOF,
		},
		{
			name: "seek over cluster bound with wrong cluster marker (EOC)",
			fs:   testingNew(t, testFileReader(fat16InvalidFiles)),
			args: args{
				cluster:  6,
				fileSize: 10513,
				offset:   0,
				readSize: 0,
			},
			want:    []byte("## GoFAT\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an example FAT32 volume used to test GoFAT.\nThis is an "),
			wantErr: io.ErrUnexpectedEOF,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := tt.fs
			got, err := fs.readFileAt(tt.args.cluster, tt.args.fileSize, tt.args.offset, tt.args.readSize)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("readFileAt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("readFileAt() got = %v, want %v", got, tt.want)
			}
		})
	}
}
