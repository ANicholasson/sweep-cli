package domain

// DockerData is the space Docker images and build cache hold, or why a
// Scan could not read it. It is never a Candidate, and a Clean never acts
// on it. A value is either read, with a Size, or unread, with a reason,
// and never both.
type DockerData struct {
	read bool
	size Size
	why  string
}

// Reported is Docker data a Scan could read. size is the images and the
// build cache together, in bytes, and it may be zero.
func Reported(size Size) DockerData {
	return DockerData{read: true, size: size}
}

// Unread is Docker data a Scan could not read. why is what a report says,
// such as when Docker is missing or not running.
func Unread(why string) DockerData {
	return DockerData{why: why}
}

// Size is the bytes images and build cache hold. The boolean is false
// when they could not be read, and then Mark says why.
func (d DockerData) Size() (Size, bool) {
	if !d.read {
		return 0, false
	}
	return d.size, true
}

// Mark is the words a report shows for this Docker data: "reported only"
// when it was read, and why it was not otherwise.
func (d DockerData) Mark() string {
	if d.read {
		return "reported only"
	}
	if d.why == "" {
		return "docker did not answer"
	}
	return d.why
}
