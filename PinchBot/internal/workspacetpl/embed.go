package workspacetpl

import "embed"

const Root = "workspace"

// Files is the canonical set of starter workspace templates used to bootstrap
// new PinchBot workspaces.
//
//go:embed workspace
var Files embed.FS
