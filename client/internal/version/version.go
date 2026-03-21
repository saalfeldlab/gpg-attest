package version

// Version is the native host version string.
// Override at build time with: go build -ldflags "-X gpg-attest.org/client/internal/version.Version=x.y.z"
var Version = "dev"
