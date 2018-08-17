/*
Sniperkit-Bot
- Status: analyzed
*/

package fstree

import (
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"github.com/sniperkit/snk.fork.gitviewfs/gitviewfs/fserror"
)

type Node interface{}

type DirNode interface {
	Node
	Children() (map[string]Node, *fserror.Error)
}

type FileNode interface {
	Node
	File() *object.File
}
