package sys

import (
	"context"
	"fmt"
	"sync"
)

// FaceEmbeddingProvider is the boundary for an optional external face engine.
// The publish addon owns media, permissions and vector search; providers only
// detect faces and return model-specific embeddings.
type FaceEmbeddingProvider interface {
	Name() string
	Version() string
	Extract(context.Context, *FaceEmbeddingRequest) (*FaceEmbeddingResponse, error)
}

type FaceEmbeddingRequest struct {
	MediaType string
	Content   []byte
	FileName  string
}

type FaceEmbedding struct {
	Index          int
	BoundingBox    []float64
	DetectionScore float64
	QualityScore   float64
	Values         []float32
}

type FaceEmbeddingResponse struct {
	Faces []*FaceEmbedding
}

var faceEmbeddingProviders = struct {
	sync.RWMutex
	items map[string]FaceEmbeddingProvider
}{items: make(map[string]FaceEmbeddingProvider)}

func registerFaceEmbeddingProvider(provider FaceEmbeddingProvider) error {
	if provider == nil || provider.Name() == "" {
		return fmt.Errorf("人脸特征提供方无效")
	}
	faceEmbeddingProviders.Lock()
	defer faceEmbeddingProviders.Unlock()
	faceEmbeddingProviders.items[provider.Name()] = provider
	return nil
}

func faceEmbeddingProvider(name string) FaceEmbeddingProvider {
	faceEmbeddingProviders.RLock()
	defer faceEmbeddingProviders.RUnlock()
	return faceEmbeddingProviders.items[name]
}

func faceEmbeddingProviderNames() []string {
	faceEmbeddingProviders.RLock()
	defer faceEmbeddingProviders.RUnlock()
	names := make([]string, 0, len(faceEmbeddingProviders.items))
	for name := range faceEmbeddingProviders.items {
		names = append(names, name)
	}
	return names
}
