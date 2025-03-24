package cli

const (
	// Download
	DownloadMinSplitKb = 5120
	DownloadSplitCount = 3

	// Upload
	UploadMinSplitMb  = 200
	UploadSplitCount  = 5
	UploadChunkSizeMb = 20

	// Common
	Retries                = 3
	ArtifactoryTokenExpiry = 3600
	RetryWaitMilliSecs     = 0
)
