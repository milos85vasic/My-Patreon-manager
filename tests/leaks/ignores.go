package leaks

import "go.uber.org/goleak"

// Ignores returns the project-wide goleak allowlist for framework-owned
// goroutines we cannot terminate (database/sql connector, gin trust proxy,
// testcontainers reaper, etc.).
func Ignores() []goleak.Option {
	return []goleak.Option{
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionResetter"),
		goleak.IgnoreTopFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect"),
	}
}
