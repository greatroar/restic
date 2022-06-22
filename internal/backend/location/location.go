package location

import (
	"strings"

	"github.com/restic/restic/internal/backend/azure"
	"github.com/restic/restic/internal/backend/b2"
	"github.com/restic/restic/internal/backend/gs"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/rclone"
	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/backend/sftp"
	"github.com/restic/restic/internal/backend/swift"
	"github.com/restic/restic/internal/errors"
)

// A location specifies the location of a repository, including the method of
// access and (possibly) credentials needed for access.
type location struct {
	Scheme string
	Config interface{}
}

type parser struct {
	scheme        string
	parse         func(string) (interface{}, error)
	stripPassword func(string) string
}

// parsers is a list of valid config parsers for the backends. The first parser
// is the fallback and should always be set to the local backend.
var parsers = []parser{
	{"b2", b2.ParseConfig, noPassword},
	{"local", local.ParseConfig, noPassword},
	{"sftp", sftp.ParseConfig, noPassword},
	{"s3", s3.ParseConfig, noPassword},
	{"gs", gs.ParseConfig, noPassword},
	{"azure", azure.ParseConfig, noPassword},
	{"swift", swift.ParseConfig, noPassword},
	{"rest", rest.ParseConfig, rest.StripPassword},
	{"rclone", rclone.ParseConfig, noPassword},
}

// noPassword returns the repository location unchanged (there's no sensitive information there)
func noPassword(s string) string {
	return s
}

func isPath(s string) bool {
	if strings.HasPrefix(s, "../") || strings.HasPrefix(s, `..\`) {
		return true
	}

	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, `\`) {
		return true
	}

	if len(s) < 3 {
		return false
	}

	// check for drive paths
	drive := s[0]
	if !(drive >= 'a' && drive <= 'z') && !(drive >= 'A' && drive <= 'Z') {
		return false
	}

	if s[1] != ':' {
		return false
	}

	if s[2] != '\\' && s[2] != '/' {
		return false
	}

	return true
}

// parseLocation extracts repository location information from the string s.
//
// If s starts with a backend name followed by a colon, that backend's Parse()
// function is called. Otherwise, the local backend is used which interprets s
// as the name of a directory.
func parseLocation(s string) (u location, err error) {
	scheme := extractScheme(s)
	u.Scheme = scheme

	for _, parser := range parsers {
		if parser.scheme != scheme {
			continue
		}

		u.Config, err = parser.parse(s)
		if err != nil {
			return location{}, err
		}

		return u, nil
	}

	// if s is not a path or contains ":", it's ambiguous
	if !isPath(s) && strings.ContainsRune(s, ':') {
		return location{}, errors.New("invalid backend\nIf the repo is in a local directory, you need to add a `local:` prefix")
	}

	u.Scheme = "local"
	u.Config, err = local.ParseConfig("local:" + s)
	if err != nil {
		return location{}, err
	}

	return u, nil
}

// StripPassword returns a displayable version of a repository location (with any sensitive information removed)
func StripPassword(s string) string {
	scheme := extractScheme(s)

	for _, parser := range parsers {
		if parser.scheme != scheme {
			continue
		}
		return parser.stripPassword(s)
	}
	return s
}

func extractScheme(s string) string {
	data := strings.SplitN(s, ":", 2)
	return data[0]
}
