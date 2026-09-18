package ghclient

import "time"

// timeoutAfterSeconds is a small helper so cycle tests fail rather than hang.
func timeoutAfterSeconds(n int) <-chan time.Time {
	return time.After(time.Duration(n) * time.Second)
}
