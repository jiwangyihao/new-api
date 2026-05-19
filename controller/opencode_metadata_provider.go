package controller

import (
	"context"

	"github.com/QuantumNous/new-api/service"
)

type openCodeMetadataProvider interface {
	GetOpenAIModels(ctx context.Context) (map[string]service.OpenCodeOpenAIModel, error)
	GetOMPProviderToolsMetadata(ctx context.Context) service.OMPProviderToolsMetadata
}

var getOpenCodeMetadataProvider = func() openCodeMetadataProvider {
	return service.GetOpenCodeMetadataService()
}
