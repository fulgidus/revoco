package engine

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
)

// HashResult holds Phase 3 deduplication output.
type HashResult struct {
	// MediaHash maps media path -> hex MD5 hash
	MediaHash map[string]string
	// Unique is the deduplicated set of media paths to keep (album copies win)
	Unique []string
	// Duplicates is the count of removed duplicate files
	Duplicates int
}

// DeduplicateFiles computes MD5 hashes for all media files and removes duplicates.
// Album files (mediaAlbum[path] != "") take priority over root files.
func DeduplicateFiles(mediaFiles map[string]string, mediaAlbum map[string]string, progress func(done, total int)) (*HashResult, error) {
	paths := make([]string, 0, len(mediaFiles))
	for p := range mediaFiles {
		paths = append(paths, p)
	}
	total := len(paths)

	// Parallel MD5 hashing with bounded goroutine pool
	type hashJob struct {
		index int
		path  string
	}
	type hashOut struct {
		path string
		hash string
		err  error
	}

	workers := runtime.NumCPU()
	jobs := make(chan hashJob, workers*4)
	results := make(chan hashOut, workers*4)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				h, err := md5File(job.path)
				results <- hashOut{path: job.path, hash: h, err: err}
			}
		}()
	}

	// Feed jobs
	go func() {
		for i, p := range paths {
			jobs <- hashJob{index: i, path: p}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	// Collect results
	mediaHash := make(map[string]string, total)
	done := 0
	for r := range results {
		if r.err == nil {
			mediaHash[r.path] = r.hash
		}
		done++
		if progress != nil {
			progress(done, total)
		}
	}

	// Deduplication: for each hash, prefer album copy over root copy
	// hash -> best path
	bestByHash := make(map[string]string, len(mediaHash))
	for path, hash := range mediaHash {
		if hash == "" {
			continue
		}
		existing, seen := bestByHash[hash]
		if !seen {
			bestByHash[hash] = path
			continue
		}
		// Album copy wins
		existingIsAlbum := mediaAlbum[existing] != ""
		thisIsAlbum := mediaAlbum[path] != ""
		if thisIsAlbum && !existingIsAlbum {
			bestByHash[hash] = path
		}
	}

	unique := make([]string, 0, len(bestByHash))
	for _, p := range bestByHash {
		unique = append(unique, p)
	}
	duplicates := total - len(unique)

	return &HashResult{
		MediaHash:  mediaHash,
		Unique:     unique,
		Duplicates: duplicates,
	}, nil
}

// md5File computes the hex MD5 hash of a file.
func md5File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
