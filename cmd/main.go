package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	g "github.com/gogits/git"
	"github.com/lemmi/ghfs"
)

func POE(err error, prefix ...interface{}) {
	if err != nil {
		log.Print(prefix...)
		log.Fatal(err)
	}
}

type gitroot struct {
	path, branchname string
}

func (gr gitroot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path, err := filepath.Abs(gr.path)
	POE(err, "Filepath")

	repo, err := g.OpenRepository(path)
	POE(err, "OpenRepository")

	commit, err := repo.GetCommitOfBranch(gr.branchname)
	POE(err, "LookupBranch")

	log.Print("On branch ", gr.branchname)
	log.Print("Serving tree of commit ", commit.Id)

	http.FileServer(ghfs.FromCommit(commit)).ServeHTTP(w, r)
}

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	path := "."
	branchname := "master"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	if len(os.Args) > 2 {
		branchname = os.Args[2]
	}

	http.ListenAndServe(":8008", gitroot{path, branchname})
}
