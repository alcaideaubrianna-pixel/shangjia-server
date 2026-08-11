package sysin

import "time"

const (
	MediaKindPhoto = "photo"
	MediaKindVideo = "video"
	MediaKindFile  = "file"

	MediaStatusProcessing = "processing"
	MediaStatusReady      = "ready"
	MediaStatusFailed     = "failed"
)

type MediaCacheEntry struct {
	Fingerprint       string    `json:"fingerprint"`
	StoragePath       string    `json:"storagePath"`
	PosterStoragePath string    `json:"posterStoragePath,omitempty"`
	PHash             string    `json:"phash,omitempty"`
	DHash             string    `json:"dhash,omitempty"`
	Kind              string    `json:"kind"`
	MimeType          string    `json:"mimeType"`
	Size              int64     `json:"size"`
	PipelineVersion   string    `json:"pipelineVersion"`
	Status            string    `json:"status"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
