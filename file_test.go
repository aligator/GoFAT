package gofat

import (
	"errors"
	"io"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
)

// fileTestFields is essentially a copy of the File struct used to fill the
// unit under test in test cases.
type fileTestFields struct {
	path         string
	isDirectory  bool
	isReadOnly   bool
	isHidden     bool
	isSystem     bool
	firstCluster fatEntry
	stat         os.FileInfo
	offset       int64
}

// fakeFileInfo is just a fake FileInfo which does nothing and contains only
// someData to have something to check equality.
type fakeFileInfo struct {
	someData string
	fileSize int64
}

func (f fakeFileInfo) Name() string       { return "" }
func (f fakeFileInfo) Size() int64        { return f.fileSize }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() interface{}   { return nil }

// fileTestsError is just a error used in tests for File.
var fileTestsError = errors.New("a super error")

func TestFile_Close(t *testing.T) {
	tests := []struct {
		name    string
		fields  fileTestFields
		wantErr bool
	}{
		{
			name: "just close and reset all fields",
			fields: fileTestFields{
				path:         "any path",
				isDirectory:  true,
				isReadOnly:   true,
				isHidden:     true,
				isSystem:     true,
				firstCluster: 5,
				stat:         entryHeaderFileInfo{},
				offset:       7,
			},
		},
	}

	fEmpty := File{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           &Fs{},
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			if err := f.Close(); (err != nil) != tt.wantErr {
				t.Errorf("File.Close() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && *f != fEmpty {
				t.Errorf("File.Close() did not reset all fields: File = %v want = %v", *f, fEmpty)
			}
		})
	}
}

func TestFile_Read(t *testing.T) {
	type args struct {
		p []byte
	}
	type mock struct {
		readAtResult []byte
		readAtError  error
	}
	tests := []struct {
		name     string
		mockData mock
		fields   fileTestFields
		args     args
		wantN    int
		wantErr  error
	}{
		{
			name: "simple file",
			mockData: mock{
				readAtResult: []byte{'H', 'e', 'l', 'l', '0', ' ', 'W', 'o', 'r', 'l', 'd'},
				readAtError:  nil,
			},
			fields: fileTestFields{
				firstCluster: 0,
				stat:         fakeFileInfo{fileSize: 11},
			},
			args: args{
				p: make([]byte, 11),
			},
			wantN:   11,
			wantErr: nil,
		},
		{
			name: "simple file with offset",
			mockData: mock{
				readAtResult: []byte{' ', 'W', 'o', 'r', 'l', 'd'},
				readAtError:  nil,
			},
			fields: fileTestFields{
				firstCluster: 0,
				offset:       5,
				stat:         fakeFileInfo{fileSize: 11},
			},
			args: args{
				p: make([]byte, 6),
			},
			wantN:   6,
			wantErr: nil,
		},
		{
			name: "error while reading",
			mockData: mock{
				readAtResult: []byte{'H'}, // Simulate error after some bytes are already read.
				readAtError:  fileTestsError,
			},
			fields: fileTestFields{
				firstCluster: 0,
				stat:         fakeFileInfo{fileSize: 11},
			},
			args: args{
				p: make([]byte, 11),
			},
			wantN:   1,
			wantErr: fileTestsError,
		},
		{
			name: "file smaller than buffer",
			mockData: mock{
				readAtResult: []byte{'H', 'e', 'l', 'l', '0', ' ', 'W', 'o', 'r', 'l', 'd'},
				readAtError:  io.EOF,
			},
			fields: fileTestFields{
				firstCluster: 0,
				stat:         fakeFileInfo{fileSize: 11},
			},
			args: args{
				p: make([]byte, 20),
			},
			wantN:   11,
			wantErr: io.EOF,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockFs := NewMockfatFileFs(mockCtrl)
			mockFs.EXPECT().
				readFileAt(tt.fields.firstCluster, tt.fields.stat.Size(), tt.fields.offset, int64(len(tt.args.p))).
				MaxTimes(1).
				Return(tt.mockData.readAtResult, tt.mockData.readAtError)

			f := &File{
				fs:           mockFs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}

			gotN, err := f.Read(tt.args.p)

			mockCtrl.Finish()

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("File.Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("File.Read() = %v, want %v", gotN, tt.wantN)
			}
		})
	}
}

func TestFile_ReadAt(t *testing.T) {
	type args struct {
		p   []byte
		off int64
	}
	type mock struct {
		readAtResult []byte
		readAtError  error
	}
	tests := []struct {
		name     string
		fields   fileTestFields
		args     args
		mockData mock
		wantN    int
		wantErr  error
	}{
		{
			name: "simple file",
			mockData: mock{
				readAtResult: []byte{'e', 'l', 'l', '0', ' ', 'W', 'o', 'r', 'l', 'd'},
				readAtError:  nil,
			},
			fields: fileTestFields{
				firstCluster: 0,
				stat:         fakeFileInfo{fileSize: 11},
			},
			args: args{
				p:   make([]byte, 10),
				off: 1,
			},
			wantN:   10,
			wantErr: nil,
		},
		{
			name: "error while reading",
			mockData: mock{
				readAtResult: nil,
				readAtError:  fileTestsError,
			},
			fields: fileTestFields{
				firstCluster: 0,
				stat:         fakeFileInfo{fileSize: 11},
			},
			args: args{
				p:   make([]byte, 11),
				off: 1,
			},
			wantN:   0,
			wantErr: fileTestsError,
		},
		{
			name: "not enough data (EOF)",
			mockData: mock{
				readAtResult: []byte{'e', 'l', 'l', '0'},
				readAtError:  io.EOF,
			},
			fields: fileTestFields{
				firstCluster: 0,
				stat:         fakeFileInfo{fileSize: 11},
			},
			args: args{
				p:   make([]byte, 10),
				off: 1,
			},
			wantN:   4,
			wantErr: io.EOF,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockFs := NewMockfatFileFs(mockCtrl)
			mockFs.EXPECT().
				readFileAt(tt.fields.firstCluster, tt.fields.stat.Size(), tt.args.off, int64(len(tt.args.p))).
				MaxTimes(1).
				Return(tt.mockData.readAtResult, tt.mockData.readAtError)

			f := &File{
				fs:           mockFs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			gotN, err := f.ReadAt(tt.args.p, tt.args.off)

			mockCtrl.Finish()

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("File.ReadAt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("File.ReadAt() = %v, want %v", gotN, tt.wantN)
			}
		})
	}
}

func TestFile_Seek(t *testing.T) {
	type args struct {
		offset int64
		whence int
	}
	tests := []struct {
		name    string
		fields  fileTestFields
		args    args
		want    int64
		wantErr bool
	}{
		{
			name: "Seek from start regardless of previous offset",
			fields: fileTestFields{
				offset: 1234,
				stat: entryHeaderFileInfo{entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						FileSize: 5000,
					},
				}},
			},
			args: args{
				offset: 100,
				whence: io.SeekStart,
			},
			want:    100,
			wantErr: false,
		},
		{
			name: "Seek from last offset",
			fields: fileTestFields{
				offset: 1000,
				stat: entryHeaderFileInfo{entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						FileSize: 5000,
					},
				}},
			},
			args: args{
				offset: 200,
				whence: io.SeekCurrent,
			},
			want:    1200,
			wantErr: false,
		},
		{
			name: "Seek from the end",
			fields: fileTestFields{
				offset: 1000,
				stat: entryHeaderFileInfo{entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						FileSize: 5000,
					},
				}},
			},
			args: args{
				offset: -200,
				whence: io.SeekEnd,
			},
			want:    4800,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			got, err := f.Seek(tt.args.offset, tt.args.whence)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.Seek() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("File.Seek() = %v, want %v", got, tt.want)
			}

			// f.offset must be set also.
			if f.offset != tt.want {
				t.Errorf("File.offset = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFile_Write(t *testing.T) {
	type args struct {
		p []byte
	}
	tests := []struct {
		name    string
		fields  fileTestFields
		args    args
		wantN   int
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			gotN, err := f.Write(tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("File.Write() = %v, want %v", gotN, tt.wantN)
			}
		})
	}
}

func TestFile_WriteAt(t *testing.T) {
	type args struct {
		p   []byte
		off int64
	}
	tests := []struct {
		name    string
		fields  fileTestFields
		args    args
		wantN   int
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			gotN, err := f.WriteAt(tt.args.p, tt.args.off)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.WriteAt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("File.WriteAt() = %v, want %v", gotN, tt.wantN)
			}
		})
	}
}

func TestFile_Name(t *testing.T) {
	// Currently not needed as it's only a pass through to stats.
}

func TestFile_Readdir(t *testing.T) {
	type args struct {
		count int
	}
	type mock struct {
		readRootResult []ExtendedEntryHeader
		readRootError  error

		readDirResult []ExtendedEntryHeader
		readDirError  error
	}
	tests := []struct {
		name     string
		fields   fileTestFields
		args     args
		mockData mock
		want     []os.FileInfo
		wantErr  error
	}{
		{
			name: "Read root dir",
			fields: fileTestFields{
				path:        "",
				isDirectory: true,
			},
			args: args{
				count: -1,
			},
			mockData: mock{
				readRootResult: []ExtendedEntryHeader{
					// Use the name to identify them in the results, they are just tested by equality.
					{ExtendedName: "1"},
					{ExtendedName: "2"},
					{ExtendedName: "3"},
				},
				readRootError: nil,
			},
			want: []os.FileInfo{
				entryHeaderFileInfo{ExtendedEntryHeader{ExtendedName: "1"}},
				entryHeaderFileInfo{ExtendedEntryHeader{ExtendedName: "2"}},
				entryHeaderFileInfo{ExtendedEntryHeader{ExtendedName: "3"}},
			},
			wantErr: nil,
		},
		{
			name: "Read dir",
			fields: fileTestFields{
				path:        "/test",
				isDirectory: true,
			},
			args: args{
				count: -1,
			},
			mockData: mock{
				readDirResult: []ExtendedEntryHeader{
					// Use the name to identify them in the results, they are just tested by equality.
					{ExtendedName: "1"},
					{ExtendedName: "2"},
					{ExtendedName: "3"},
				},
				readRootError: nil,
			},
			want: []os.FileInfo{
				entryHeaderFileInfo{ExtendedEntryHeader{ExtendedName: "1"}},
				entryHeaderFileInfo{ExtendedEntryHeader{ExtendedName: "2"}},
				entryHeaderFileInfo{ExtendedEntryHeader{ExtendedName: "3"}},
			},
			wantErr: nil,
		},
		{
			name: "Read dir with count arg",
			fields: fileTestFields{
				path:        "/test",
				isDirectory: true,
			},
			args: args{
				count: 2,
			},
			mockData: mock{
				readDirResult: []ExtendedEntryHeader{
					// Use the name to identify them in the results, they are just tested by equality.
					{ExtendedName: "1"},
					{ExtendedName: "2"},
					{ExtendedName: "3"},
				},
				readRootError: nil,
			},
			want: []os.FileInfo{
				entryHeaderFileInfo{ExtendedEntryHeader{ExtendedName: "1"}},
				entryHeaderFileInfo{ExtendedEntryHeader{ExtendedName: "2"}},
			},
			wantErr: nil,
		},
		{
			name: "No dir",
			fields: fileTestFields{
				path:        "/test",
				isDirectory: false,
			},
			args: args{
				count: -1,
			},
			mockData: mock{},
			want:     nil,
			wantErr:  syscall.ENOTDIR,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockFs := NewMockfatFileFs(mockCtrl)

			if tt.mockData.readDirResult != nil {
				mockFs.EXPECT().
					readDir(tt.fields.firstCluster).
					MaxTimes(1).
					Return(tt.mockData.readDirResult, tt.mockData.readDirError)
			}

			if tt.mockData.readRootResult != nil {
				mockFs.EXPECT().
					readRoot().
					MaxTimes(1).
					Return(tt.mockData.readRootResult, tt.mockData.readRootError)
			}

			f := &File{
				fs:           mockFs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			got, err := f.Readdir(tt.args.count)

			mockCtrl.Finish()

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("File.Readdir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("File.Readdir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFile_Readdirnames(t *testing.T) {
	type args struct {
		count int
	}
	type mock struct {
		readRootResult []ExtendedEntryHeader
		readRootError  error

		readDirResult []ExtendedEntryHeader
		readDirError  error
	}
	tests := []struct {
		name     string
		fields   fileTestFields
		args     args
		mockData mock
		want     []string
		wantErr  error
	}{
		{
			name: "Read root dir",
			fields: fileTestFields{
				path:        "",
				isDirectory: true,
			},
			args: args{
				count: -1,
			},
			mockData: mock{
				readRootResult: []ExtendedEntryHeader{
					// Use the name to identify them in the results, they are just tested by equality.
					{ExtendedName: "1"},
					{ExtendedName: "2"},
					{ExtendedName: "3"},
				},
				readRootError: nil,
			},
			want:    []string{"1", "2", "3"},
			wantErr: nil,
		},
		{
			name: "Read dir",
			fields: fileTestFields{
				path:        "/test",
				isDirectory: true,
			},
			args: args{
				count: -1,
			},
			mockData: mock{
				readDirResult: []ExtendedEntryHeader{
					// Use the name to identify them in the results, they are just tested by equality.
					{ExtendedName: "1"},
					{ExtendedName: "2"},
					{ExtendedName: "3"},
				},
				readRootError: nil,
			},
			want:    []string{"1", "2", "3"},
			wantErr: nil,
		},
		{
			name: "Read dir with count arg",
			fields: fileTestFields{
				path:        "/test",
				isDirectory: true,
			},
			args: args{
				count: 2,
			},
			mockData: mock{
				readDirResult: []ExtendedEntryHeader{
					// Use the name to identify them in the results, they are just tested by equality.
					{ExtendedName: "1"},
					{ExtendedName: "2"},
					{ExtendedName: "3"},
				},
				readRootError: nil,
			},
			want:    []string{"1", "2"},
			wantErr: nil,
		},
		{
			name: "No dir",
			fields: fileTestFields{
				path:        "/test",
				isDirectory: false,
			},
			args: args{
				count: 0,
			},
			mockData: mock{},
			want:     nil,
			wantErr:  syscall.ENOTDIR,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockFs := NewMockfatFileFs(mockCtrl)

			if tt.mockData.readDirResult != nil {
				mockFs.EXPECT().
					readDir(tt.fields.firstCluster).
					MaxTimes(1).
					Return(tt.mockData.readDirResult, tt.mockData.readDirError)
			}

			if tt.mockData.readRootResult != nil {
				mockFs.EXPECT().
					readRoot().
					MaxTimes(1).
					Return(tt.mockData.readRootResult, tt.mockData.readRootError)
			}

			f := &File{
				fs:           mockFs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			got, err := f.Readdirnames(tt.args.count)

			mockCtrl.Finish()

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("File.Readdirnames() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("File.Readdirnames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFile_Stat(t *testing.T) {
	tests := []struct {
		name    string
		fields  fileTestFields
		want    os.FileInfo
		wantErr bool
	}{
		{
			name: "simple stats",
			fields: fileTestFields{
				stat: fakeFileInfo{someData: "1"},
			},
			want: fakeFileInfo{someData: "1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			got, err := f.Stat()
			if (err != nil) != tt.wantErr {
				t.Errorf("File.Stat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("File.Stat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFile_Sync(t *testing.T) {
	tests := []struct {
		name    string
		fields  fileTestFields
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			if err := f.Sync(); (err != nil) != tt.wantErr {
				t.Errorf("File.Sync() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFile_Truncate(t *testing.T) {
	type args struct {
		size int64
	}
	tests := []struct {
		name    string
		fields  fileTestFields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			if err := f.Truncate(tt.args.size); (err != nil) != tt.wantErr {
				t.Errorf("File.Truncate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFile_WriteString(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		fields  fileTestFields
		args    args
		wantRet int
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			gotRet, err := f.WriteString(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.WriteString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotRet != tt.wantRet {
				t.Errorf("File.WriteString() = %v, want %v", gotRet, tt.wantRet)
			}
		})
	}
}
