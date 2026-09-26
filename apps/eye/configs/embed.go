// Package configs embeds the default source registry and correlation rules into
// the binary.
//
// eye ships as one file you copy. A binary that needs a YAML file next to it in
// order to do anything is two files, so the shipped registry travels inside it
// and a user-supplied file overrides it when present.
package configs

import _ "embed"

// DefaultSources is the source registry shipped with this build.
//
//go:embed sources.yaml
var DefaultSources []byte

// DefaultRules is the correlation rule set shipped with this build.
//
//go:embed rules.yaml
var DefaultRules []byte
