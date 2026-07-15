//go:build !darwin

package profile

// linkSystemKeychain is a no-op outside macOS: other platforms have no
// home-anchored system keychain, so agents fall back to the shared
// file-based credential links.
func linkSystemKeychain(string) error {
	return nil
}
