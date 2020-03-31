package gitfs

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"gopkg.in/src-d/go-billy.v4"
	"gopkg.in/src-d/go-billy.v4/memfs"
	"gopkg.in/src-d/go-billy.v4/osfs"
	"gopkg.in/src-d/go-billy.v4/util"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing/cache"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	gogitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
	"gopkg.in/src-d/go-git.v4/storage/filesystem"
)

// Not thread safe
type Git struct {
	ctx     context.Context
	repoUrl string
	auth    *gogitssh.PublicKeys
	fs      billy.Filesystem
	repo    *git.Repository
	wt      *git.Worktree
	pulled  bool
}

func NewGit(ctx context.Context, repoUrl string, useMemfs bool, baseDir string, errorIfExists bool) (*Git, error) {
	sshKey, err := ioutil.ReadFile(fmt.Sprintf("%s/.ssh/id_rsa", os.Getenv("HOME")))
	if err != nil {
		return nil, errors.Wrapf(err, "error reading private key")
	}

	signer, err := ssh.ParsePrivateKey([]byte(sshKey))
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing private key")
	}
	auth := &gogitssh.PublicKeys{User: "git", Signer: signer}

	var fs billy.Filesystem
	if useMemfs {
		fs = memfs.New()
	} else {
		fs = osfs.New(baseDir)
	}

	dotStore, exists, err := buildDotStore(fs, errorIfExists)
	if err != nil {
		return nil, errors.Wrapf(err, "error chrooting .git")
	}

	var repo *git.Repository
	if exists {
		repo, err = git.Open(dotStore, fs)
	} else {
		repo, err = git.CloneContext(
			ctx,
			dotStore,
			fs,
			&git.CloneOptions{
				URL:      repoUrl,
				Auth:     auth,
				Progress: os.Stdout,
			})
	}

	if err != nil {
		return nil, errors.Wrapf(err, "error cloning repo %v", repoUrl)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, errors.Wrapf(err, "error reading worktree")
	}

	return &Git{
		repoUrl: repoUrl,
		auth:    auth,
		repo:    repo,
		wt:      wt,
		fs:      fs,
		pulled:  false,
	}, nil
}

func buildDotStore(fs billy.Filesystem, errorIfExists bool) (*filesystem.Storage, bool, error) {
	fi, err := fs.Stat(git.GitDirName)
	exists := !os.IsNotExist(err)
	if err != nil && !os.IsNotExist(err) {
		return nil, exists, err
	}

	if exists {
		if errorIfExists {
			return nil, true, errors.New("repo already exists")
		}
		if !fi.IsDir() {
			return nil, true, errors.New(".git is not a directory")
		}
	}

	dot, err := fs.Chroot(git.GitDirName)
	if err != nil {
		return nil, exists, errors.Wrapf(err, "error chrooting %v", git.GitDirName)
	}
	return filesystem.NewStorage(dot, cache.NewObjectLRUDefault()), exists, nil
}

func (g *Git) Reset() error {
	if err := util.RemoveAll(g.fs, git.GitDirName); err != nil {
		return errors.Wrapf(err, "error removing .git")
	}

	dotStore, _, err := buildDotStore(g.fs, true)
	if err != nil {
		return err
	}

	repo, err := git.Init(dotStore, g.fs)
	if err != nil {
		return errors.Wrapf(err, "error initing repo")
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{g.repoUrl},
	})
	if err != nil {
		return errors.Wrapf(err, "error pushing change")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return errors.Wrapf(err, "error reading worktree")
	}

	g.repo = repo
	g.wt = wt

	return nil
}

func (g *Git) FileSystem() billy.Filesystem {
	return g.fs
}

func (g *Git) Pull() error {
	if err := g.wt.Pull(&git.PullOptions{
		RemoteName: "origin",
		Auth:       g.auth,
		Progress:   os.Stdout,
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		return errors.Wrapf(err, "error pulling changes from origin")
	}

	return nil
}

func traverseDir(fs billy.Filesystem, dir string, cb func(string) error) error {
	fmt.Printf("traverse dir %v\n", dir)

	files, err := fs.ReadDir(dir)
	if err != nil {
		fmt.Printf("read dir err: %v\n", err)
		return err
	}

	for _, fi := range files {
		if fi.Name() == git.GitDirName {
			continue
		}

		path := filepath.Join(dir, fi.Name())
		fmt.Printf("dir %v, name %v, path %v\n", dir, fi.Name(), path)
		if fi.IsDir() {
			if err := traverseDir(fs, path, cb); err != nil {
				return err
			}
		} else {
			if err := cb(path[1:]); err != nil {
				return err
			}
		}
	}

	return nil
}

func (g *Git) AddAll() error {
	_, err := g.wt.Add("")
	return err
}

func (g *Git) Commit(msg string) error {
	_, err := g.wt.Commit(msg, &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  "gitfs",
			Email: "gitfs@github.com",
			When:  time.Now(),
		},
	})
	return err
}

func (g *Git) Push() error {
	return g.repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			"+refs/heads/master:refs/heads/master",
		},
		Auth:     g.auth,
		Progress: os.Stdout,
	})
}

const (
	branchNamePrefix = "refs/heads/"
)

type StatusCode byte

const (
	Unmodified         StatusCode = ' '
	Inconsistent       StatusCode = '!'
	Untracked          StatusCode = '?'
	Modified           StatusCode = 'M'
	Added              StatusCode = 'A'
	Deleted            StatusCode = 'D'
	Renamed            StatusCode = 'R'
	Copied             StatusCode = 'C'
	UpdatedButUnmerged StatusCode = 'U'
)

func (g *Git) GetStatus() (map[string]StatusCode, error) {
	s, err := g.wt.Status()
	if err != nil {
		return nil, errors.Wrapf(err, "error getting status")
	}
	fmt.Printf("status: len %v, %v\n", len(s), s.String())
	files := map[string]StatusCode{}
	if err := traverseDir(g.fs, "/", func(path string) error {
		fstatus := s[path]
		if fstatus == nil {
		} else if fstatus.Staging == git.Unmodified && fstatus.Worktree == git.Unmodified {
		} else if fstatus.Staging == git.Unmodified {
			files[path] = StatusCode(byte(fstatus.Worktree))
		} else if fstatus.Worktree == git.Unmodified {
			files[path] = StatusCode(byte(fstatus.Staging))
		} else if fstatus.Staging != fstatus.Worktree {
			files[path] = Inconsistent
		} else {
			files[path] = StatusCode(byte(fstatus.Worktree))
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return files, nil
}
