package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iamjinlei/gitfs"
)

func main() {
	// a git url that you have permission to operate
	// such as git@github.com:iamjinlei/gitfs.git
	url := flag.String("url", "", "remote repo url")
	memfs := flag.Bool("memfs", true, "use memfs")
	flag.Parse()

	repoUrl := strings.TrimSpace(*url)
	if repoUrl == "" {
		fmt.Printf("url cannot be empty\n")
		return
	}

	var c *gitfs.Config
	if *memfs {
		fmt.Printf("using memfs")
		c = gitfs.NewConfig().SetUrl(repoUrl).UseMemFs()
	} else {
		base := "/tmp/gitfs_osfs"
		os.MkdirAll(base, 0664)
		fmt.Printf("using os fs %v\n", base)
		c = gitfs.NewConfig().SetUrl(repoUrl).UseOsFs(base, true)
	}

	fmt.Printf("creating GitFs\n")
	fs, err := gitfs.New(context.TODO(), c)
	if err != nil {
		fmt.Printf("error creating GitFs %v\n", err)
		return
	}

	fmt.Printf("pulling from remote\n")
	if err := fs.Pull(); err != nil {
		fmt.Printf("error pulling repo %v\n", err)
		return
	}

	// overwrite test file content
	fmt.Printf("overwriting file overwrite.txt\n")
	f, err := fs.Create("overwrite.txt")
	if err != nil {
		fmt.Printf("error creating file %v\n", err)
		return
	}
	f.Write([]byte(time.Now().Format("2006-01-02T15:04:05")))
	f.Close()

	_, err = fs.Stat("add_del.txt")
	if os.IsNotExist(err) {
		fmt.Printf("adding file add_del.txt\n")

		f, err := fs.Create("add_del.txt")
		if err != nil {
			fmt.Printf("error creating file %v\n", err)
			return
		}
		f.Write([]byte("added file"))
		f.Close()
	} else {
		fmt.Printf("deleting file add_del.txt\n")

		if err := fs.RemoveAll("add_del.txt"); err != nil {
			fmt.Printf("error removing file %v\n", err)
			return
		}
	}

	fmt.Printf("syncing fs to remote\n")
	if err := fs.Sync(true); err != nil {
		fmt.Printf("error syncing fs %v\n", err)
	}
}
