// Package buildinfo identifies the running build.
package buildinfo

// Version is the release this binary was built from. A release build stamps
// it with -ldflags "-X github.com/gatework/motor/common/buildinfo.Version=…";
// the default is the release the source tree was last tagged as.
var Version = "2.4.1"
