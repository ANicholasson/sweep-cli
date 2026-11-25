// Package scan looks for Candidates. A Scan changes nothing. It reads the
// file tree only through Files, it asks the Git rule only through
// safety.Git, and it reads Docker data only through Docker. None of the
// ports has a method that writes.
package scan

import "time"

// Files is the port a Scan reads the file tree through. It only reads, and
// it never follows a Link: a Link is listed and measured as itself, and what
// it leads to is never listed.
//
// A path is whatever the adapter understands. A Scan makes none of its own:
// it starts from a Root and from then on passes back only an Entry's Path.
// So the domain holds no separator and no drive letter, and it behaves the
// same over the fake on every operating system.
type Files interface {
	// List names what is directly inside dir, in order of Name.
	List(dir string) ([]Entry, error)

	// Space reports the disk space of the one file, directory or Link at
	// path: its own, not what a directory holds and not what a Link leads
	// to.
	Space(path string) (Space, error)

	// Modified reports the modification time of the one file, directory or
	// Link at path. It never reports an access time.
	Modified(path string) (time.Time, error)
}

// Entry is one name inside a directory that was listed.
type Entry struct {
	Name string
	// Path is what List and Space take to reach this Entry.
	Path string
	Is   Is
}

// Is says what an Entry is. A Link is never also a Dir or a File, whatever
// it leads to, so a walk that enters only a Dir never follows one.
type Is uint8

const (
	// File is a regular file.
	File Is = iota + 1
	// Dir is a real directory.
	Dir
	// Link is anything that makes the path lead somewhere else: a symlink,
	// a Windows junction, or another reparse point sweep does not know.
	Link
	// Other is whatever is left, such as a socket or a device.
	Other
)

// Space is the disk space of one path, and which file it is.
type Space struct {
	// Bytes is what the file system allocated, a multiple of 512, which is
	// the block du counts in. It is not the file's length.
	Bytes int64
	// File is the same for every hard link to one file and differs between
	// two files. It is never the zero FileID.
	File FileID
}

// FileID tells one file from another on one machine.
type FileID struct {
	Volume uint64
	File   uint64
}
