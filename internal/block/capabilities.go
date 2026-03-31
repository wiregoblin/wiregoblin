// Package block contains runtime contracts shared by workflow blocks and the engine.
package block

// Closer is implemented by blocks that hold open connections.
type Closer interface {
	Close()
}

// ResponseMappingProvider advertises whether a block supports response mapping rows.
type ResponseMappingProvider interface {
	SupportsResponseMapping() bool
}

// ReferencePolicyProvider describes which block fields accept constants and/or runtime variables.
// Read-only built-ins in the ! namespace follow the same policy gates.
type ReferencePolicyProvider interface {
	ReferencePolicy() []ReferencePolicy
}
