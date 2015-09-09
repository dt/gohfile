package hfile

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
)

type CollectionConfig struct {
	// The Name of the collection.
	Name string

	// The Hfile itself.
	SourcePath string

	// A local copy of SourcePath, if SourcePath is remote, otherwise the same as SourcePath.
	LocalPath string

	// If the collection data should be kept in-memory (via mlock).
	InMem bool

	// Should operations on this collection emit verbose debug output.
	Debug bool
}

type CollectionSet struct {
	Collections map[string]*Reader
	cache       string
}

func LoadCollections(collections []*CollectionConfig, cache string) (*CollectionSet, error) {
	cs := new(CollectionSet)
	cs.Collections = make(map[string]*Reader)

	if len(collections) < 1 {
		return nil, fmt.Errorf("no collections to load!")
	}

	if err := downloadCollections(collections, cache); err != nil {
		log.Println("[LoadCollections] Error fetching collections: ", err)
		return nil, err
	}

	for _, cfg := range collections {
		reader, err := NewReaderFromConfig(*cfg)
		if err != nil {
			return nil, err
		}

		cs.Collections[cfg.Name] = reader
	}

	return cs, nil
}

func downloadCollections(collections []*CollectionConfig, cache string) error {
	for _, cfg := range collections {
		if cfg.LocalPath == "" {
			cfg.LocalPath = cfg.SourcePath
		}

		remote := isRemote(cfg.SourcePath)
		if remote {
			cfg.LocalPath = localCache(cfg.SourcePath, cache)
			if _, err := os.Stat(cfg.LocalPath); err == nil {
				if cfg.Debug {
					log.Printf("[FetchRemote] %s already cached: %s.", cfg.Name, cfg.LocalPath)
				}
			} else if !os.IsNotExist(err) {
				return err
			} else {
				err = fetch(cfg)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func isRemote(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

func localCache(url, cache string) string {
	h := md5.Sum([]byte(url))
	name := hex.EncodeToString(h[:]) + ".hfile"
	return path.Join(cache, name)
}

func fetch(cfg *CollectionConfig) error {
	log.Printf("[FetchRemote] Fetching %s: %s -> %s.", cfg.Name, cfg.SourcePath, cfg.LocalPath)

	fp, err := os.Create(cfg.LocalPath)
	if err != nil {
		return err
	}
	defer fp.Close()

	resp, err := http.Get(cfg.SourcePath)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		return fmt.Errorf("HTTP error fetching (%s): %s\n", resp.Status, buf.String())
	}
	defer resp.Body.Close()

	sz, err := io.Copy(fp, resp.Body)
	if err != nil {
		return err
	}

	sz = sz / (1024 * 1024)
	log.Printf("[FetchRemote] Fetched %s (%dmb).", cfg.Name, sz)
	return nil
}

func (cs *CollectionSet) ReaderFor(name string) (*Reader, error) {
	c, ok := cs.Collections[name]
	if !ok {
		return nil, fmt.Errorf("not configured with reader for collection %s", name)
	}
	return c, nil
}
