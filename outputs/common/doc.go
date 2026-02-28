// Package common provides shared output modules that work across services.
//
// All outputs in this package auto-register themselves via init() functions.
// Simply import this package to make all outputs available:
//
//	import _ "github.com/fulgidus/revoco/outputs/common"
//
// Available outputs:
//   - local-folder: Copy/move files to local directory
//   - immich: Upload to Immich photo server
//   - photoprism: Upload to PhotoPrism server
//   - s3: Upload to S3-compatible storage
//   - google-photos-api: Upload to Google Photos via API
package common
