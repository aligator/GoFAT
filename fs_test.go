package gofat

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func fat32TestFileReader() io.ReadSeeker {
	fsFile, err := os.Open("./testdata/fat32.img")
	if err != nil {
		fmt.Println("Make sure you ran go generate.")
		panic(err)
	}

	return fsFile
}

func fat16TestFileReader() io.ReadSeeker {
	fsFile, err := os.Open("./testdata/fat16.img")
	if err != nil {
		fmt.Println("Make sure you ran go generate.")
		panic(err)
	}

	return fsFile
}

func fat32TooSmallTestFileReader() io.ReadSeeker {
	fsFile, err := os.Open("./testdata/fat32-invalid-sectors-per-cluster.img")
	if err != nil {
		fmt.Println("Make sure you ran go generate.")
		panic(err)
	}

	return fsFile
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
				reader: fat32TestFileReader(),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "FAT16 test image",
			args: args{
				reader: fat16TestFileReader(),
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
				reader: fat32TooSmallTestFileReader(),
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
				reader: fat32TestFileReader(),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "FAT16 test image",
			args: args{
				reader: fat16TestFileReader(),
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
				reader: fat32TooSmallTestFileReader(),
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

type fatEntryTests struct {
	name  string
	eFrom fatEntry
	eTo   fatEntry
	want  bool
}

func testFatEntry(t *testing.T, tests []fatEntryTests, method string, execute func(e fatEntry) bool) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for e := tt.eFrom; e <= tt.eTo; e++ {
				if got := execute(e); got != tt.want {
					t.Errorf("fatEntry(0x%x).%v() = %v, want %v", e, method, got, tt.want)
				}
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
		lock        sync.Mutex
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        tt.fields.lock,
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
		lock        sync.Mutex
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	tests := []struct {
		name   string
		fields fields
		want   FATType
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        tt.fields.lock,
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
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
	type fields struct {
		lock        sync.Mutex
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
		want    afero.File
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        tt.fields.lock,
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			got, err := fs.Open(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fs.Open() error = %v, wantErr %v", err, tt.wantErr)
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
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
	type fields struct {
		lock        sync.Mutex
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
		want    os.FileInfo
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        tt.fields.lock,
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			got, err := fs.Stat(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fs.Stat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Fs.Stat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFs_Name(t *testing.T) {
	type fields struct {
		lock        sync.Mutex
		reader      io.ReadSeeker
		info        Info
		sectorCache Sector
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &Fs{
				lock:        tt.fields.lock,
				reader:      tt.fields.reader,
				info:        tt.fields.info,
				sectorCache: tt.fields.sectorCache,
			}
			if got := fs.Name(); got != tt.want {
				t.Errorf("Fs.Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFs_Chmod(t *testing.T) {
	type fields struct {
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
		lock        sync.Mutex
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
				lock:        tt.fields.lock,
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
