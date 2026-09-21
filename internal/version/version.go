// Package version carries enzo's version number, so the binary, the usage text
// and the commits enzo writes all name the same thing.
package version

import "fmt"

// Version is enzo's release, as MAJOR.MINOR.PATCH.
//
// The value here is the source of truth. A build from a git tag overrides it
// with -ldflags "-X github.com/justin-efficient/enzo/internal/version.Version=..."
// so a tagged binary names its tag; see the Makefile.
var Version = "0.5.0"

// Banner is how enzo names itself and its version — in `enzo --version`, at the
// top of the usage text, and under the issue list.
func Banner() string { return fmt.Sprintf("🚘 enzo v%s", Version) }

// Credit is how enzo signs what it leaves behind: the commit at the root of a
// branch `enzo start` made, and the footer of an issue enzo opened. It answers
// "what put this here?" wherever something turns up with no author.
func Credit() string { return "Created by " + Banner() }
