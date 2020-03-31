package gitfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/src-d/go-billy.v4"
	"gopkg.in/src-d/go-billy.v4/util"
)

type Config struct {
	// Remote repo url. Only support ssh protocol
	repoUrl string
	// If use local memory to back filesystem
	useMemFs bool
	// If use OS file system (not memory fs), then provide dir path
	osFsBaseDir string
	// If open existing repo
	openExisting bool
}

func NewConfig() *Config {
	return &Config{}
}

func (c *Config) SetUrl(url string) *Config {
	c.repoUrl = url
	return c
}

func (c *Config) UseMemFs() *Config {
	c.useMemFs = true
	c.openExisting = false
	c.osFsBaseDir = ""
	return c
}

func (c *Config) UseOsFs(baseDir string, openExisting bool) *Config {
	c.useMemFs = false
	c.openExisting = openExisting
	c.osFsBaseDir = baseDir
	return c
}

func (c *Config) Valid() error {
	c.repoUrl = strings.TrimSpace(c.repoUrl)
	if c.repoUrl == "" {
		return errors.New("empty repo url")
	}

	c.osFsBaseDir = strings.TrimSpace(c.osFsBaseDir)
	if c.useMemFs && c.osFsBaseDir != "" {
		return errors.New("memFs and osFs base dir are mutually exclusive")
	} else if !c.useMemFs && c.osFsBaseDir == "" {
		return errors.New("osFs base dir is not provided")
	}

	return nil
}

func New(ctx context.Context, config *Config) (*GitFs, error) {
	if err := config.Valid(); err != nil {
		return nil, err
	}

	git, err := NewGit(ctx, config.repoUrl, config.useMemFs, config.osFsBaseDir, !config.openExisting)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating git client")
	}

	return &GitFs{
		git: git,
		fs:  git.FileSystem(),
	}, nil
}

type GitFs struct {
	git *Git
	fs  billy.Filesystem
}

func (g *GitFs) Pull() error {
	return g.git.Pull()
}

func (g *GitFs) Sync(purge bool) error {
	if purge {
		if err := g.git.Reset(); err != nil {
			return errors.Wrapf(err, "error resetting git")
		}
	}

	if err := g.git.AddAll(); err != nil {
		return errors.Wrapf(err, "error adding files to git")
	}

	if err := g.git.Commit(fmt.Sprintf("gitfs sync - %v", time.Now().Format("2006-01-02T15:04:05Z07:00"))); err != nil {
		return errors.Wrapf(err, "error committing sync changes")
	}

	/* TODO: currently merge is not supported by go-git
	if err := g.git.Pull(); err != nil {
		return errors.Wrapf(err, "error pulling change from remote repo")
	}
	*/

	if err := g.git.Push(); err != nil {
		return errors.Wrapf(err, "error pushing change to remote repo")
	}
	return nil
}

// --- Below are standard fs operations ---
type File interface {
	// Name returns the name of the file as presented to Open.
	Name() string
	io.Writer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
	// Lock locks the file like e.g. flock. It protects against access from
	// other processes.
	Lock() error
	// Unlock unlocks the file.
	Unlock() error
	// Truncate the file.
	Truncate(size int64) error
}

// Create creates the named file with mode 0666 (before umask), truncating
// it if it already exists. If successful, methods on the returned File can
// be used for I/O; the associated file descriptor has mode O_RDWR.
func (g *GitFs) Create(filename string) (File, error) {
	return g.fs.Create(filename)
}

// Open opens the named file for reading. If successful, methods on the
// returned file can be used for reading; the associated file descriptor has
// mode O_RDONLY.
func (g *GitFs) Open(filename string) (File, error) {
	return g.fs.Open(filename)
}

// OpenFile is the generalized open call; most users will use Open or Create
// instead. It opens the named file with specified flag (O_RDONLY etc.) and
// perm, (0666 etc.) if applicable. If successful, methods on the returned
// File can be used for I/O.
func (g *GitFs) OpenFile(filename string, flag int, perm os.FileMode) (File, error) {
	return g.fs.OpenFile(filename, flag, perm)
}

// Stat returns a FileInfo describing the named file.
func (g *GitFs) Stat(filename string) (os.FileInfo, error) {
	return g.fs.Stat(filename)
}

// Rename renames (moves) oldpath to newpath. If newpath already exists and
// is not a directory, Rename replaces it. OS-specific restrictions may
// apply when oldpath and newpath are in different directories.
func (g *GitFs) Rename(oldpath, newpath string) error {
	return g.fs.Rename(oldpath, newpath)
}

// Remove removes the named file or directory.
func (g *GitFs) Remove(filename string) error {
	return g.fs.Remove(filename)
}

// RemoveAll removes the named file or directory including sub-directories.
func (g *GitFs) RemoveAll(path string) error {
	return util.RemoveAll(g.fs, path)
}

// Join joins any number of path elements into a single path, adding a
// Separator if necessary. Join calls filepath.Clean on the result; in
// particular, all empty strings are ignored. On Windows, the result is a
// UNC path if and only if the first path element is a UNC path.
func (g *GitFs) Join(elem ...string) string {
	return g.fs.Join(elem...)
}

// TempFile creates a new temporary file in the directory dir with a name
// beginning with prefix, opens the file for reading and writing, and
// returns the resulting *os.File. If dir is the empty string, TempFile
// uses the default directory for temporary files (see os.TempDir).
// Multiple programs calling TempFile simultaneously will not choose the
// same file. The caller can use f.Name() to find the pathname of the file.
// It is the caller's responsibility to remove the file when no longer
// needed.
func (g *GitFs) TempFile(dir, prefix string) (File, error) {
	return g.fs.TempFile(dir, prefix)
}

// ReadDir reads the directory named by dirname and returns a list of
// directory entries sorted by filename.
func (g *GitFs) ReadDir(path string) ([]os.FileInfo, error) {
	return g.fs.ReadDir(path)
}

// MkdirAll creates a directory named path, along with any necessary
// parents, and returns nil, or else returns an error. The permission bits
// perm are used for all directories that MkdirAll creates. If path is/
// already a directory, MkdirAll does nothing and returns nil.
func (g *GitFs) MkdirAll(filename string, perm os.FileMode) error {
	return g.fs.MkdirAll(filename, perm)
}

// Lstat returns a FileInfo describing the named file. If the file is a
// symbolic link, the returned FileInfo describes the symbolic link. Lstat
// makes no attempt to follow the link.
func (g *GitFs) Lstat(filename string) (os.FileInfo, error) {
	return g.fs.Lstat(filename)
}

// Symlink creates a symbolic-link from link to target. target may be an
// absolute or relative path, and need not refer to an existing node.
// Parent directories of link are created as necessary.
func (g *GitFs) Symlink(target, link string) error {
	return g.fs.Symlink(target, link)
}

// Readlink returns the target path of link.
func (g *GitFs) Readlink(link string) (string, error) {
	return g.fs.Readlink(link)
}

// Chroot returns a new filesystem from the same type where the new root is
// the given path. Files outside of the designated directory tree cannot be
// accessed.
func (g *GitFs) Chroot(path string) (*GitFs, error) {
	fs, err := g.fs.Chroot(path)
	if err != nil {
		return nil, err
	}
	return &GitFs{fs: fs}, nil
}

// Root returns the root path of the filesystem.
func (g *GitFs) Root() string {
	return g.fs.Root()
}

func (g *GitFs) Exist(path string) (bool, error) {
	_, err := g.fs.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "error stating path")
	}

	return true, nil
}
