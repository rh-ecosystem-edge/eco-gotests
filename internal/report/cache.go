package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/klauspost/compress/zstd"
	"k8s.io/klog/v2"
)

const (
	cacheDir = "eco-gotests"
)

var (
	errCacheMiss = fmt.Errorf("cache miss")
)

// IsMiss returns true if the given error is a cache miss error and false otherwise.
func IsMiss(err error) bool {
	return errors.Is(err, errCacheMiss)
}

// CacheKey holds the information necessary to index the cached trees. Using both the branch and revision allows for
// easily verifying if the tree cached for a given branch is current.
type CacheKey struct {
	Branch   string
	Revision string
}

// Cache represents the format of the cache file. It will be saved as JSON according to the XDG base directory
// specification.
type Cache struct {
	Trees     map[CacheKey]*SuiteTree
	directory string
	ctx       context.Context
}

// NewCacheContext creates a new cache instance. It will attempt to load the cache from the cache file. If the file does
// not exist, a new cache will be created but not saved until Save is called.
func NewCacheContext(ctx context.Context) (*Cache, error) {
	klog.V(100).Info("Instantiating new Cache and attempting to load")

	cache := &Cache{
		Trees: make(map[CacheKey]*SuiteTree),
		ctx:   ctx,
	}

	err := cache.Load()
	if err != nil {
		return nil, err
	}

	return cache, nil
}

// CleanCache cleans the existing cache on disk by removing the entire eco-gotests cache directory.
func CleanCache() error {
	cache := &Cache{ctx: context.TODO()}

	cachePath, err := cache.getDirectory()
	if err != nil {
		return err
	}

	klog.V(100).Infof("Deleting cache directory at %s", cachePath)

	err = os.RemoveAll(cachePath)
	if err != nil {
		return err
	}

	return nil
}

// Load will attempt to load all regular files from the cache directory then update the cache if necessary. It tries to
// ignore files that may cause an error. It will return early on errors, leaving the cache in an invalid state until
// Update is called.
func (cache *Cache) Load() error {
	cachePath, err := cache.getDirectory()
	if err != nil {
		return err
	}

	klog.V(100).Infof("Loading cache from %s", cachePath)

	if sourceCodeSum == "" {
		klog.V(100).Info("Unable to retrieve source code sum. Cache entries cannot be verified and will not be loaded.")

		return nil
	}

	cacheDirEntries, err := os.ReadDir(cachePath)
	if err != nil {
		klog.V(100).Info("Unable to access cache directory. Cache entries will not be loaded.")

		return nil
	}

	for _, dirEntry := range cacheDirEntries {
		klog.V(100).Infof("Considering loading cache from %s", dirEntry.Name())

		// We only ever create regular files, so all other files should be ignored.
		if !dirEntry.Type().IsRegular() {
			klog.V(100).Infof("Not going to load cache from %s because it is not a regular file", dirEntry.Name())

			continue
		}

		key, sum := parseCacheFileName(dirEntry.Name())
		if sum != sourceCodeSum {
			continue
		}

		tree, err := loadCacheFile(filepath.Join(cachePath, dirEntry.Name()))
		if err != nil {
			return err
		}

		cache.Trees[key] = tree
	}

	return cache.Update()
}

// Save saves the cache to the cache directory, returning early on any errors.
func (cache *Cache) Save() error {
	if sourceCodeSum == "" {
		klog.V(100).Info("Unable to retrieve source code sum. Cache entries will not be saved.")

		return nil
	}

	cachePath, err := cache.getDirectory()
	if err != nil {
		return err
	}

	klog.V(100).Infof("Saving cache with %d trees to %s", len(cache.Trees), cachePath)

	err = os.MkdirAll(cachePath, 0755)
	if err != nil {
		return err
	}

	err = cache.deleteExpiredFiles()
	if err != nil {
		return err
	}

	for key, tree := range cache.Trees {
		err := saveCacheFile(filepath.Join(cachePath, generateCacheFileName(key)), tree)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetRemotePatterns fetches the branches from the remote repository that match patterns. For each match, the branch and
// revision are concatenated and used as the key in the returned map. If the match was present in the cache, then its
// value is the cached SuiteTree. If the match was not present in the cache, its value is nil. All matches will appear
// in the returned map.
func (cache *Cache) GetRemotePatterns(patterns []string) (map[CacheKey]*SuiteTree, error) {
	klog.V(100).Infof("Checking if branches matching patterns %v are in cache", patterns)

	if sourceCodeSum == "" {
		klog.V(100).Info("Unable to retrieve source code sum. All cache lookups will miss.")

		return nil, errCacheMiss
	}

	revisions, err := GetRemoteRevisions(cache.ctx, RemoteURL, slices.Values(patterns))
	if err != nil {
		return nil, err
	}

	cachedTrees := make(map[CacheKey]*SuiteTree)

	for branch, revision := range revisions {
		key := CacheKey{Branch: branch, Revision: revision}

		tree, ok := cache.Trees[key]
		if !ok {
			cachedTrees[key] = nil
		} else {
			cachedTrees[key] = tree
		}
	}

	return cachedTrees, nil
}

// Get returns the suite tree for the given repo path from the cache. It returns a cache miss error if the repo has
// uncommitted changes or if the cache does not contain the repo.
func (cache *Cache) Get(repoPath string) (*SuiteTree, error) {
	klog.V(100).Infof("Getting cache for repo %s", repoPath)

	if sourceCodeSum == "" {
		klog.V(100).Info("Unable to retrieve source code sum. All cache lookups will miss.")

		return nil, errCacheMiss
	}

	key, err := cache.GetKeyFromPath(repoPath)
	if err != nil {
		return nil, err
	}

	tree, ok := cache.Trees[key]
	if !ok {
		return nil, errCacheMiss
	}

	return tree, nil
}

// GetOrCreate returns the suite tree for the given repo path from the cache. It first calls Get and if there is a cache
// miss, it calls the given create function and adds the result to the cache. Note that if the repo has local changes,
// the create function will always be called, but the result will not be added to the cache.
func (cache *Cache) GetOrCreate(repoPath string) (*SuiteTree, error) {
	klog.V(100).Infof("Getting or creating cache for repo %s", repoPath)

	tree, err := cache.Get(repoPath)
	if err == nil {
		return tree, nil
	}

	if !IsMiss(err) {
		return nil, err
	}

	klog.V(100).Infof("Cache miss for repo %s, dry running on repo", repoPath)

	reportPath, err := DryRun(cache.ctx, repoPath)
	if err != nil {
		klog.V(100).Infof("Failed to run eco-gotests dry-run: %v", err)

		return nil, err
	}

	tree, err = NewFromFile(reportPath)
	if err != nil {
		klog.V(100).Infof("Failed to create SuiteTree from report.json: %v", err)

		return nil, err
	}

	_ = os.Remove(reportPath)

	key, err := cache.GetKeyFromPath(repoPath)
	if err == nil {
		cache.Trees[key] = tree
	} else {
		klog.V(100).Infof("Failed to get cache key for repo %s, created tree not saved", repoPath)
	}

	return tree, nil
}

// Update iterates through all the cached trees and deletes any that are out of date. Entries are considered out of
// date when the branch name and hash do not match the remote repo or when the checksum does not match this module's
// checksum.
func (cache *Cache) Update() error {
	klog.V(100).Info("Updating cache and removing out of date entries")

	if sourceCodeSum == "" {
		klog.V(100).Info(
			"Unable to retrieve source code sum. All cache entries will be removed as their validity cannot be verified.")

		cache.Trees = make(map[CacheKey]*SuiteTree)

		return nil
	}

	cachedRevisions := make(map[string]string)

	for key := range cache.Trees {
		cachedRevisions[key.Branch] = key.Revision
	}

	remoteRevisions, err := GetRemoteRevisions(cache.ctx, RemoteURL, maps.Keys(cachedRevisions))
	if err != nil {
		return err
	}

	for cachedBranch, cachedRevision := range cachedRevisions {
		remoteRevision, ok := remoteRevisions[cachedBranch]
		if ok && cachedRevision == remoteRevision {
			continue
		}

		delete(cache.Trees, CacheKey{Branch: cachedBranch, Revision: cachedRevision})
	}

	return nil
}

// GetKeyFromPath returns the cache file name that corresponds to the repo at repoPath. It returns a cache miss error if
// the repo has uncommitted changes and a different error if no source code sum is available.
func (cache *Cache) GetKeyFromPath(repoPath string) (CacheKey, error) {
	klog.V(100).Infof("Getting cache key for repo %s", repoPath)

	changes, err := HasLocalChanges(cache.ctx, repoPath)
	if err != nil {
		return CacheKey{}, err
	}

	if changes {
		klog.V(100).Infof("Repo %s has uncommitted changes, cache will always miss", repoPath)

		return CacheKey{}, errCacheMiss
	}

	branch, err := GetRepoBranch(cache.ctx, repoPath)
	if err != nil {
		return CacheKey{}, err
	}

	revision, err := GetRepoRevision(cache.ctx, repoPath)
	if err != nil {
		return CacheKey{}, err
	}

	return CacheKey{Branch: branch, Revision: revision}, nil
}

// getDirectory returns the stored directory for this cache if it exists, otherwise it uses a subdirectory of the
// OS-specific user cache directory. If the user cache directory does not exist, it returns an error.
func (cache *Cache) getDirectory() (string, error) {
	if cache.directory != "" {
		return cache.directory, nil
	}

	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	cachePath := filepath.Join(userCache, cacheDir)

	return cachePath, nil
}

// deleteExpiredFiles will delete all regular files in the cache directory that are not included in the current cache.
// It returns early on any error deleting files.
func (cache *Cache) deleteExpiredFiles() error {
	cachePath, err := cache.getDirectory()
	if err != nil {
		return err
	}

	klog.V(100).Infof("Deleting expired cache files from %s", cachePath)

	cacheDirEntries, err := os.ReadDir(cachePath)
	if err != nil {
		return err
	}

	for _, dirEntry := range cacheDirEntries {
		// We only create regular files so avoid deleting anything not created by us.
		if !dirEntry.Type().IsRegular() {
			continue
		}

		key, sum := parseCacheFileName(dirEntry.Name())

		_, keyFound := cache.Trees[key]
		if keyFound && sum == sourceCodeSum {
			continue
		}

		err := os.RemoveAll(dirEntry.Name())
		if err != nil {
			return err
		}
	}

	return nil
}

// saveCacheFile saves the tree at the path provided by cacheFileName, truncating if the file already exists.
func saveCacheFile(cacheFileName string, tree *SuiteTree) error {
	klog.V(100).Infof("Saving cached tree to %s", cacheFileName)

	file, err := os.Create(cacheFileName)
	if err != nil {
		return err
	}

	defer file.Close()

	compressor, err := zstd.NewWriter(file)
	if err != nil {
		return err
	}

	defer compressor.Close()

	err = json.NewEncoder(compressor).Encode(tree)
	if err != nil {
		return err
	}

	return nil
}

// loadCacheFile attempts to load a SuiteTree from cacheFileName.
func loadCacheFile(cacheFileName string) (*SuiteTree, error) {
	klog.V(100).Infof("Loading cached tree from %s", cacheFileName)

	file, err := os.Open(cacheFileName)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	decompressor, err := zstd.NewReader(file)
	if err != nil {
		return nil, err
	}

	tree := &SuiteTree{}

	err = json.NewDecoder(decompressor).Decode(tree)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// generateCacheFileName takes the parameters and generates the corresponding cache file name. It guarantees that the
// name consists of branch, revision, and sum joined by spaces. The extension should be considered opaque.
func generateCacheFileName(key CacheKey) string {
	return fmt.Sprintf("%s %s %s.json.zstd", key.Branch, key.Revision, sourceCodeSum)
}

// parseCacheFileName takes the cacheFileName and extracts the branch, revision, and sum. It is guaranteed to be the
// inverse of generateCacheFileName. For an invalid cacheFileName, all returns will be empty.
func parseCacheFileName(cacheFileName string) (key CacheKey, sum string) {
	withoutExtension, _ := strings.CutSuffix(cacheFileName, ".json.zstd")
	fields := strings.Fields(withoutExtension)

	if len(fields) != 3 {
		return CacheKey{}, ""
	}

	return CacheKey{Branch: fields[0], Revision: fields[1]}, fields[2]
}
