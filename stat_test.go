package gofat

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestExtendedEntryHeader_FileInfo(t *testing.T) {
	type fields struct {
		EntryHeader  EntryHeader
		ExtendedName string
	}
	tests := []struct {
		name   string
		fields fields
		want   os.FileInfo
	}{
		{
			name: "it just has to be the same",
			fields: fields{
				EntryHeader: EntryHeader{
					Name:            [11]byte{'H', 'E', 'L', 'L', 'O', ' ', ' ', ' ', 'T', 'X', 'T'},
					Attribute:       AttrDirectory,
					NTReserved:      0,
					CreateTimeTenth: 1,
					CreateTime:      2,
					CreateDate:      3,
					LastAccessDate:  4,
					FirstClusterHI:  5,
					WriteTime:       6,
					WriteDate:       7,
					FirstClusterLO:  8,
					FileSize:        9,
				},
				ExtendedName: "huhu",
			},
			want: entryHeaderFileInfo{
				entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Name:            [11]byte{'H', 'E', 'L', 'L', 'O', ' ', ' ', ' ', 'T', 'X', 'T'},
						Attribute:       AttrDirectory,
						NTReserved:      0,
						CreateTimeTenth: 1,
						CreateTime:      2,
						CreateDate:      3,
						LastAccessDate:  4,
						FirstClusterHI:  5,
						WriteTime:       6,
						WriteDate:       7,
						FirstClusterLO:  8,
						FileSize:        9,
					},
					ExtendedName: "huhu",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &ExtendedEntryHeader{
				EntryHeader:  tt.fields.EntryHeader,
				ExtendedName: tt.fields.ExtendedName,
			}
			if got := h.FileInfo(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtendedEntryHeader.FileInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_entryHeaderFileInfo_Name(t *testing.T) {
	type fields struct {
		entry ExtendedEntryHeader
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "only 8.3 filename",
			fields: fields{
				ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Name: [11]byte{'H', 'E', 'L', 'L', 'O', ' ', ' ', ' ', 'T', 'X', 'T'},
					},
					ExtendedName: "",
				},
			},
			want: "HELLO.TXT",
		},
		{
			name: "only 8.3 short extension",
			fields: fields{
				ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Name: [11]byte{'H', 'E', 'L', 'L', 'O', ' ', ' ', ' ', 'T', 'X', ' '},
					},
					ExtendedName: "",
				},
			},
			want: "HELLO.TX",
		},
		{
			name: "only 8.3 no extension",
			fields: fields{
				ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Name: [11]byte{'H', 'E', 'L', 'L', 'O', ' ', ' ', ' ', ' ', ' ', ' '},
					},
					ExtendedName: "",
				},
			},
			want: "HELLO",
		},
		{
			name: "only 8.3 no extension",
			fields: fields{
				ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Name: [11]byte{'H', 'E', 'L', 'L', 'O', ' ', ' ', ' ', ' ', ' ', ' '},
					},
					ExtendedName: "",
				},
			},
			want: "HELLO",
		},
		{
			name: "with extended filename",
			fields: fields{
				ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Name: [11]byte{'H', 'E', 'L', 'L', 'O', 'W', '~', '1', 'T', 'X', 'T'},
					},
					ExtendedName: "HelloWorldThisIsALoongFileName.txt",
				},
			},
			want: "HelloWorldThisIsALoongFileName.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entryHeaderFileInfo{
				entry: tt.fields.entry,
			}
			if got := e.Name(); got != tt.want {
				t.Errorf("entryHeaderFileInfo.Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_entryHeaderFileInfo_Size(t *testing.T) {
	type fields struct {
		entry ExtendedEntryHeader
	}
	tests := []struct {
		name   string
		fields fields
		want   int64
	}{
		{
			name: "some size",
			fields: fields{
				entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						FileSize: 5555,
					},
				},
			},
			want: 5555,
		},
		{
			name: "zero size",
			fields: fields{
				entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						FileSize: 0,
					},
				},
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entryHeaderFileInfo{
				entry: tt.fields.entry,
			}
			if got := e.Size(); got != tt.want {
				t.Errorf("entryHeaderFileInfo.Size() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_entryHeaderFileInfo_Mode(t *testing.T) {
	type fields struct {
		entry ExtendedEntryHeader
	}
	tests := []struct {
		name   string
		fields fields
		want   os.FileMode
	}{
		{
			name: "No directory",
			fields: fields{
				entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Attribute: 0,
					},
				},
			},
			want: 0,
		},
		{
			name: "Directory",
			fields: fields{
				entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Attribute: AttrDirectory,
					},
				},
			},
			want: os.ModeDir,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entryHeaderFileInfo{
				entry: tt.fields.entry,
			}
			if got := e.Mode(); got != tt.want {
				t.Errorf("entryHeaderFileInfo.Mode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_entryHeaderFileInfo_ModTime(t *testing.T) {
	type fields struct {
		entry ExtendedEntryHeader
	}
	tests := []struct {
		name   string
		fields fields
		want   time.Time
	}{
		{
			name: "a normal write time and date",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       41936,
					WriteDate:       20890,
				},
			}},
			want: time.Date(2020, 12, 26, 20, 30, 32, 0, time.UTC),
		},
		{
			name: "a zero write time and date results in time.Time.IsZero() == true",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       0,
					WriteDate:       0,
				},
			}},
			want: time.Time{},
		},
		{
			name: "a zero write time results in 00:00:00.000000000",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       0,
					WriteDate:       20890,
				},
			}},
			want: time.Date(2020, 12, 26, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "a zero write date results in time.Time.IsZero() == true as it's not specified by the specification",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       41936,
					WriteDate:       0,
				},
			}},
			want: time.Time{},
		},
		{
			name: "a zero write day results in time.Time.IsZero() == true as it's not specified by the specification",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       41936,
					WriteDate:       20928,
				},
			}},
			want: time.Time{},
		},
		{
			name: "a zero write month results in time.Time.IsZero() == true as it's not specified by the specification",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       41936,
					WriteDate:       20480,
				},
			}},
			want: time.Time{},
		},
		{
			name: "a month > 12 increases the year",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       41936,
					WriteDate:       20922,
				},
			}},
			want: time.Date(2021, 1, 26, 20, 30, 32, 0, time.UTC),
		},
		{
			name: "a second > 59 increases the minutes",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       41951,
					WriteDate:       20890,
				},
			}},
			want: time.Date(2020, 12, 26, 20, 31, 02, 0, time.UTC),
		},
		{
			name: "a minute > 59 increases the hours",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       42992,
					WriteDate:       20890,
				},
			}},
			want: time.Date(2020, 12, 26, 21, 3, 32, 0, time.UTC),
		},
		{
			name: "a time > 23:59:59 gets limited to 23:59:59",
			fields: fields{entry: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					CreateTimeTenth: 0,
					CreateTime:      0,
					CreateDate:      0,
					LastAccessDate:  0,
					WriteTime:       51199,
					WriteDate:       20890,
				},
			}},
			want: time.Date(2020, 12, 26, 23, 59, 59, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entryHeaderFileInfo{
				entry: tt.fields.entry,
			}
			if got := e.ModTime(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("entryHeaderFileInfo.ModTime() = %v, want %v", got, tt.want)
			}
			if got := e.ModTime().IsZero(); got != tt.want.IsZero() {
				t.Errorf("entryHeaderFileInfo.ModTime().IsZero() = %v, want.IsZero() %v", got, tt.want.IsZero())
			}
		})
	}
}

func Test_entryHeaderFileInfo_IsDir(t *testing.T) {
	type fields struct {
		entry ExtendedEntryHeader
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "No directory",
			fields: fields{
				entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Attribute: 0,
					},
				},
			},
			want: false,
		},
		{
			name: "Directory",
			fields: fields{
				entry: ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Attribute: AttrDirectory,
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entryHeaderFileInfo{
				entry: tt.fields.entry,
			}
			if got := e.IsDir(); got != tt.want {
				t.Errorf("entryHeaderFileInfo.IsDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_entryHeaderFileInfo_Sys(t *testing.T) {
	type fields struct {
		entry ExtendedEntryHeader
	}
	tests := []struct {
		name   string
		fields fields
		want   interface{}
	}{
		{
			name: "any header",
			fields: fields{
				ExtendedEntryHeader{
					EntryHeader: EntryHeader{
						Name: [11]byte{'H', 'E', 'L', 'L', 'O', ' ', ' ', ' ', 'T', 'X', 'T'},
					},
					ExtendedName: "AnyHeader",
				},
			},
			want: ExtendedEntryHeader{
				EntryHeader: EntryHeader{
					Name: [11]byte{'H', 'E', 'L', 'L', 'O', ' ', ' ', ' ', 'T', 'X', 'T'},
				},
				ExtendedName: "AnyHeader",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entryHeaderFileInfo{
				entry: tt.fields.entry,
			}
			if got := e.Sys(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("entryHeaderFileInfo.Sys() = %v, want %v", got, tt.want)
			}
		})
	}
}
