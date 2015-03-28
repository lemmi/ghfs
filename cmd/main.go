package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/lemmi/ghfs"
	g "gopkg.in/libgit2/git2go.v22"
)

func POE(err error, prefix ...interface{}) {
	if err != nil {
		log.Print(prefix...)
		log.Fatal(err)
	}
}

func main() {
	path := "."
	branchname := "master"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	if len(os.Args) > 2 {
		branchname = os.Args[2]
	}
	path, err := filepath.Abs(path)
	POE(err, "Filepath")

	r, err := g.OpenRepository(path)
	POE(err, "OpenRepository")
	defer r.Free()

	b, err := r.LookupBranch(branchname, g.BranchLocal)
	POE(err, "LookupBranch")
	defer b.Free()

	log.Print("On branch ", branchname)

	commit, err := r.LookupCommit(b.Target())
	POE(err, "LookupCommit")
	defer commit.Free()
	log.Print("Serving tree of commit ", commit.Id())

	tree, err := commit.Tree()
	POE(err, "Tree from commit")
	defer tree.Free()

	http.ListenAndServe(":8080", http.FileServer(ghfs.FromTree(tree)))
}
