package controller

import "testing"

func SetOpenCodeMetadataProviderForTest(t *testing.T, provider openCodeMetadataProvider) {
	t.Helper()
	old := getOpenCodeMetadataProvider
	getOpenCodeMetadataProvider = func() openCodeMetadataProvider { return provider }
	t.Cleanup(func() { getOpenCodeMetadataProvider = old })
}
