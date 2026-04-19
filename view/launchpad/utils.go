package launchpad

// This file is the home for cross-domain launchpad helpers — shared LED
// color palettes, grid-math helpers, etc. Nothing universal lives here
// yet; domain-specific helpers stay inside their own per-domain files
// (drum.go, piano.go, metropolix.go, session.go, settings.go, project.go,
// empty.go).
//
// The top-level pad/LED dispatcher lives in launchpad.go.
// The Launchpad X hardware driver lives in hw.go.
// The Mock surface (for tests) lives in mock.go.
// The Surface interface + PadEvent + LED types live in surface.go.
