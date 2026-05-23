package codex

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelListDoesNotExposeCompactSuffixModels(t *testing.T) {
	assert.Contains(t, ModelList, "gpt-5.4")
	for _, model := range ModelList {
		assert.Falsef(t, strings.HasSuffix(model, "-openai-compact"), "ModelList must not expose compact billing model %q", model)
	}
}
