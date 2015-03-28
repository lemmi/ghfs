package ghfs

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	g "gopkg.in/libgit2/git2go.v22"
)

type ghfsFileInfo struct {
	name string
	mode os.FileMode
	size int64
}

func NewFileInfoFromBlob(name string, blob *g.Blob) os.FileInfo {
	return ghfsFileInfo{name: name, size: blob.Size()}
}
func NewFileInfo(tree *g.Tree, entry *g.TreeEntry) os.FileInfo {
	ret := ghfsFileInfo{name: entry.Name}
	if entry.Filemode == g.FilemodeTree {
		ret.mode = os.ModeDir
	} else if blob, err := tree.Owner().LookupBlob(entry.Id); err == nil {
		ret.size = blob.Size()
		blob.Free()
	}
	return ret
}

// base name of the file
func (fi ghfsFileInfo) Name() string {
	return fi.name
}

// length in bytes for regular files; system-dependent for others
func (fi ghfsFileInfo) Size() int64 {
	return fi.size
}

// file mode bits
func (fi ghfsFileInfo) Mode() os.FileMode {
	return fi.mode
}

// modification time
func (fi ghfsFileInfo) ModTime() time.Time {
	return time.Now()
}

// abbreviation for Mode().IsDir()
func (fi ghfsFileInfo) IsDir() bool {
	return fi.Mode().IsDir()
}

// underlying data source (can return nil)
func (fi ghfsFileInfo) Sys() interface{} {
	return nil
}

type ghfsDir struct {
	tree *g.Tree
	fi   os.FileInfo
	idx  uint64
}

// Implement http.File on a git tree
func NewDir(tree *g.Tree, entry *g.TreeEntry) (http.File, error) {
	t, err := tree.Owner().LookupTree(entry.Id)
	if err != nil {
		return nil, err
	}
	return &ghfsDir{tree: t, fi: NewFileInfo(tree, entry)}, nil
}
func (d *ghfsDir) Read([]byte) (int, error) {
	return 0, io.EOF
}
func (d *ghfsDir) Close() error {
	d.tree.Free()
	d.tree = nil
	return nil
}
func (d *ghfsDir) Readdir(count int) ([]os.FileInfo, error) {
	if d.idx >= d.tree.EntryCount() {
		return nil, io.EOF
	}
	ret := []os.FileInfo{}

	for ; d.idx < d.tree.EntryCount(); d.idx++ {
		if count > 0 && len(ret) >= count {
			break
		}
		entry := d.tree.EntryByIndex(d.idx)
		ret = append(ret, NewFileInfo(d.tree, entry))
	}
	return ret, nil
}
func (d *ghfsDir) Seek(int64, int) (int64, error) {
	return 0, syscall.EISDIR
}
func (d *ghfsDir) Stat() (os.FileInfo, error) {
	return d.fi, nil
}

type ghfsFile struct {
	fi os.FileInfo
	r  *bytes.Reader
}

// Implement http.File on a git blob
func NewFile(tree *g.Tree, entry *g.TreeEntry) (http.File, error) {
	blob, err := tree.Owner().LookupBlob(entry.Id)
	if err != nil {
		return nil, err
	}
	defer blob.Free()
	return &ghfsFile{fi: NewFileInfoFromBlob(entry.Name, blob), r: bytes.NewReader(blob.Contents())}, nil
}

func (f *ghfsFile) Read(buf []byte) (int, error) {
	return f.r.Read(buf)
}
func (f *ghfsFile) Close() error {
	f.r = nil
	return nil
}
func (f *ghfsFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, os.ErrInvalid
}
func (f *ghfsFile) Seek(offset int64, whence int) (int64, error) {
	return f.r.Seek(offset, whence)
}
func (f *ghfsFile) Stat() (os.FileInfo, error) {
	return f.fi, nil
}

// Implement a http.Filesystem for a git Tree
type ghfs struct {
	tree *g.Tree
}

func FromTree(tree *g.Tree) http.FileSystem {
	return ghfs{tree}
}

func (fs ghfs) Open(name string) (http.File, error) {
	log.Print("Access: ", name)
	if strings.HasPrefix(name, "/") {
		name = name[1:]
	}
	var entry *g.TreeEntry
	if name == "" {
		entry = &g.TreeEntry{
			Name:     "",
			Type:     g.ObjectTree,
			Id:       fs.tree.Id(),
			Filemode: g.FilemodeTree,
		}
	} else {
		var err error
		entry, err = fs.tree.EntryByPath(name)
		if err != nil {
			log.Print(err)
			return nil, err
		}
	}

	switch entry.Type {
	case g.ObjectTree:
		return NewDir(fs.tree, entry)
	case g.ObjectBlob:
		return NewFile(fs.tree, entry)
	default:
		return nil, errors.New("Invalid type")
	}
}
