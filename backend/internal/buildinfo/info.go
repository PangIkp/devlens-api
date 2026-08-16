package buildinfo

import "fmt"

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func Summary() string {
	return fmt.Sprintf("version=%s commit=%s build_time=%s", Version, Commit, BuildTime)
}
