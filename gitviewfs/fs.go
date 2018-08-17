/*
Sniperkit-Bot
- Status: analyzed
*/

package gitviewfs

import (
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/filemode"

	"github.com/sniperkit/snk.fork.gitviewfs/gitviewfs/fserror"
	"github.com/sniperkit/snk.fork.gitviewfs/gitviewfs/fstree"
	"github.com/sniperkit/snk.fork.gitviewfs/gitviewfs/gitfstree"
)

type gitviewfs struct {
	pathfs.FileSystem
	fstree fstree.Node
	logger *log.Logger
}

func New(repo *git.Repository) (pathfs.FileSystem, error) {
	tree, err := gitfstree.New(repo)
	if err != nil {
		return nil, err
	}

	return &gitviewfs{
		FileSystem: pathfs.NewDefaultFileSystem(),
		fstree:     tree,
		logger:     log.New(ioutil.Discard, "gitviewfs", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile|log.LUTC),
	}, nil
}

func (f *gitviewfs) String() string {
	// TODO(josh-newman): Add repository path.
	return "gitviewfs"
}

func (f *gitviewfs) SetDebug(debug bool) {
	if debug {
		f.logger.SetOutput(os.Stderr)
	} else {
		f.logger.SetOutput(ioutil.Discard)
	}
}

func (f *gitviewfs) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	node, ferr := f.findNode(name)
	if ferr != nil {
		if ferr.UnexpectedErr != nil {
			f.logger.Printf("unexpected error: %s", ferr.UnexpectedErr)
		}
		return nil, ferr.Status
	}

	var attr fuse.Attr
	switch n := node.(type) {
	case fstree.DirNode:
		attr.Mode = fuse.S_IFDIR | 0555
	case fstree.FileNode:
		file := newFile(n, f.logger)
		if status := file.GetAttr(&attr); status != fuse.OK {
			return nil, status
		}
	default:
		f.logger.Printf("skipping node: %v", node)
		return nil, fuse.ENOENT
	}

	return &attr, fuse.OK
}

func (f *gitviewfs) OpenDir(name string, context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	node, ferr := f.findNode(name)
	if ferr != nil {
		if ferr.UnexpectedErr != nil {
			f.logger.Printf("unexpected error: %s", ferr.UnexpectedErr)
		}
		return nil, ferr.Status
	}

	dirNode, ok := node.(fstree.DirNode)
	if !ok {
		return nil, fuse.ENOTDIR
	}

	children, ferr := dirNode.Children()
	if ferr != nil {
		if ferr.UnexpectedErr != nil {
			f.logger.Printf("unexpected error: %s", ferr.UnexpectedErr)
		}
		return nil, ferr.Status
	}

	var entries []fuse.DirEntry
	for name, child := range children {
		entry := fuse.DirEntry{Name: name}
		switch n := child.(type) {
		case fstree.DirNode:
			entry.Mode = fuse.S_IFDIR | 0555
		case fstree.FileNode:
			if mode := computeFuseFileMode(n.File().Mode); mode != 0 {
				entry.Mode = mode
			} else {
				f.logger.Printf("skipping file child: %v", node)
			}
		default:
			f.logger.Printf("skipping child: %v", node)
		}
		entries = append(entries, entry)
	}

	return entries, fuse.OK
}

func (f *gitviewfs) Open(name string, flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	node, ferr := f.findNode(name)
	if ferr != nil {
		if ferr.UnexpectedErr != nil {
			f.logger.Printf("unexpected error: %s", ferr.UnexpectedErr)
		}
		return nil, ferr.Status
	}

	fileNode, ok := node.(fstree.FileNode)
	if !ok {
		return nil, fuse.EINVAL
	}

	return newFile(fileNode, f.logger), fuse.OK
}

func (f *gitviewfs) Readlink(name string, context *fuse.Context) (string, fuse.Status) {
	node, ferr := f.findNode(name)
	if ferr != nil {
		if ferr.UnexpectedErr != nil {
			f.logger.Printf("unexpected error: %s", ferr.UnexpectedErr)
		}
		return "", ferr.Status
	}

	fileNode, ok := node.(fstree.FileNode)
	if !ok {
		f.logger.Printf("expected file node at: %s", name)
		return "", fuse.EINVAL
	}

	if fileNode.File().Mode != filemode.Symlink {
		f.logger.Printf("expected symlink at: %s", name)
		return "", fuse.EINVAL
	}

	reader, err := fileNode.File().Reader()
	if err != nil {
		f.logger.Printf("error creating file reader: %s", err)
		return "", fuse.EIO
	}
	defer reader.Close()

	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		f.logger.Printf("error reading file: %s", err)
		return "", fuse.EIO
	}

	return string(bytes), fuse.OK
}

// computeFuseFileMode returns the (always non-zero) FUSE-suitable file mode corresponding to the
// git filemode.FileMode, or 0 indicating we should skip this file.
func computeFuseFileMode(mode filemode.FileMode) uint32 {
	switch mode {
	case filemode.Regular:
		return fuse.S_IFREG | 0444
	case filemode.Symlink:
		return fuse.S_IFLNK | 0444
	case filemode.Executable:
		return fuse.S_IFREG | 0555
	default:
		return 0
	}
}

func (f *gitviewfs) findNode(name string) (fstree.Node, *fserror.Error) {
	node := f.fstree
	if name == "" {
		return node, nil
	}
	for _, part := range strings.Split(name, "/") {
		if dirNode, ok := node.(fstree.DirNode); ok {
			children, ferr := dirNode.Children()
			if ferr != nil {
				return nil, ferr
			}

			if child, ok := children[part]; !ok {
				return nil, fserror.Expected(fuse.ENOENT)
			} else {
				node = child
			}
		} else {
			return nil, fserror.Expected(fuse.ENOENT)
		}
	}
	return node, nil
}
