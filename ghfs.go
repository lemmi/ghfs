package ghfs

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	g "github.com/lemmi/git"
)

type ghfsDir struct {
	tree    *g.Tree
	fi      os.FileInfo
	scanner *g.TreeScanner
}

// Implement http.File on a git tree
func NewDir(tree *g.Tree, fi os.FileInfo) (http.File, error) {
	scanner, err := tree.Scanner()
	if err != nil {
		return nil, err
	}
	return &ghfsDir{tree: tree, fi: fi, scanner: scanner}, nil
}
func (d *ghfsDir) Read([]byte) (int, error) {
	return 0, io.EOF
}
func (d *ghfsDir) Close() error {
	return nil
}
func (d *ghfsDir) Readdir(count int) ([]os.FileInfo, error) {
	ret := []os.FileInfo{}
	for c := 0; c < count; c++ {
		if !d.scanner.Scan() {
			break
		}
		ret = append(ret, d.scanner.TreeEntry())
	}
	if err := d.scanner.Err(); err != nil {
		return ret, err
	}
	return ret, nil
}
func (d *ghfsDir) Seek(int64, int) (int64, error) {
	return 0, syscall.EISDIR
}
func (d *ghfsDir) Stat() (os.FileInfo, error) {
	return d.fi, nil
}

type rootFileInfo struct{}

func (r rootFileInfo) Name() string {
	return ""
}
func (r rootFileInfo) Size() int64 {
	return 0
}
func (r rootFileInfo) Mode() os.FileMode {
	return os.ModeDir | 0755
}
func (r rootFileInfo) ModTime() time.Time {
	return time.Now()
}
func (r rootFileInfo) IsDir() bool {
	return true
}
func (r rootFileInfo) Sys() interface{} {
	return nil
}

type modTimeFileInfo struct {
	os.FileInfo
	modTime time.Time
}

func (m modTimeFileInfo) ModTime() time.Time {
	return m.modTime
}

type ghfsFile struct {
	entry *g.TreeEntry
	fi    os.FileInfo
	rc    io.ReadCloser
	off   int64
	atEnd bool
}

// Implement http.File on a git blob
func NewFile(entry *g.TreeEntry, fi os.FileInfo) (http.File, error) {
	return &ghfsFile{entry: entry, fi: fi}, nil
}

func (f *ghfsFile) Read(buf []byte) (int, error) {
	var err error
	if f.rc == nil {
		f.rc, err = f.entry.Blob().Data()
		if err != nil {
			return 0, err
		}
	}

	if f.atEnd {
		return 0, io.EOF
	}

	n, err := f.rc.Read(buf)
	f.off += int64(n)
	return n, err
}
func (f *ghfsFile) Close() error {
	var ret error
	if f.rc != nil {
		ret = f.rc.Close()
		f.rc = nil
	}
	f.off = 0
	return ret
}
func (f *ghfsFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, os.ErrInvalid
}
func (f *ghfsFile) Seek(offset int64, whence int) (int64, error) {
	var noff int64

	if whence == os.SEEK_CUR && f.atEnd {
		whence = os.SEEK_END
	}

	switch whence {
	case os.SEEK_SET:
		noff = offset
	case os.SEEK_CUR:
		noff = f.off + offset
	case os.SEEK_END:
		if offset == 0 {
			f.atEnd = true
			return f.entry.Size(), nil
		}
		noff = f.entry.Size() + offset
	default:
		return 0, errors.New("Invalid argument for whence")
	}

	f.atEnd = false

	switch {
	case noff < 0:
		return 0, errors.New("Invalid offset")
	case noff < f.off:
		f.Close()
		return io.CopyN(ioutil.Discard, f, noff)
	case noff >= f.off:
		_, err := io.CopyN(ioutil.Discard, f, noff-f.off)
		return f.off, err
	default:
		panic("Unreachable")
	}
}
func (f *ghfsFile) Stat() (os.FileInfo, error) {
	return f.fi, nil
}

// Implement a http.Filesystem for a git Tree
type ghfs struct {
	commit *g.Commit
	tree   *g.Tree
}

// Serve git tree from commit. Optionally from subtree
func FromCommit(commit *g.Commit, tree ...*g.Tree) http.FileSystem {
	var t *g.Tree
	if len(tree) == 0 {
		t = &commit.Tree
	} else {
		t = tree[0]
	}
	return ghfs{commit, t}
}

func (fs ghfs) Open(name string) (http.File, error) {
	var entry *g.TreeEntry
	if strings.HasPrefix(name, "/") {
		name = name[1:]
	}
	if name == "" {
		return NewDir(fs.tree, rootFileInfo{})
	} else {
		var err error
		entry, err = fs.tree.GetTreeEntryByPath(name)
		if err != nil {
			return nil, err
		}
	}

	fi := modTimeFileInfo{entry, fs.commit.Author.When}

	switch entry.Type {
	case g.ObjectTree:
		stree, err := fs.tree.SubTree(name)
		if err != nil {
			return nil, err
		}
		return NewDir(stree, fi)
	case g.ObjectBlob:
		return NewFile(entry, fi)
	default:
		return nil, errors.New("Invalid type")
	}
}
