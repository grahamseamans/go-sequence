// Package empty is the no-op device for tracks with Type == DeviceNone.
//
// There is no Compile or mutation logic here — UI handlers for the empty
// focus live under view/{launchpad,tui}/empty.go. This package remains as
// a domain marker so the tree is parallel to the other devices.
package empty
