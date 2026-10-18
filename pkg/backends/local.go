/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package backends

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/macaroni-os/anise-repo-devkit/pkg/specs"

	"github.com/klauspost/compress/zstd"
	. "github.com/macaroni-os/anise/pkg/logger"
	artifact "github.com/macaroni-os/anise/pkg/v2/compiler/types/artifact"
)

type BackendLocal struct {
	Specs *specs.AniseRDConfig
	Path  string
}

func NewBackendLocal(specs *specs.AniseRDConfig, path string) (*BackendLocal, error) {
	if path == "" {
		return nil, errors.New("Invalid path")
	}

	_, err := os.Stat(path)
	if err != nil {
		return nil, errors.New(
			fmt.Sprintf(
				"Error on retrieve stat of the path %s: %s",
				path, err.Error(),
			))
	}

	if os.IsNotExist(err) {
		return nil, errors.New("The path doesn't exist!")
	}

	ans := &BackendLocal{
		Path: path,
	}

	return ans, nil
}

func (b *BackendLocal) GetFilesList() ([]string, error) {
	ans := []string{}

	files, err := ioutil.ReadDir(b.Path)
	if err != nil {
		return ans, err
	}

	for _, f := range files {
		DebugC("Cheking file ", f.Name())
		if f.IsDir() {
			// Ignoring directories at the moment.
			DebugC(fmt.Sprintf("Ignoring directory %s", f.Name()))
			continue
		}

		ans = append(ans, f.Name())
	}

	return ans, nil
}

func (b *BackendLocal) GetMetadata(file string) (*artifact.PackageArtifact, error) {
	metafile := filepath.Join(b.Path, file)
	content, err := ioutil.ReadFile(metafile)
	if err != nil {
		return nil, errors.New(
			fmt.Sprintf("Error on open file %s", metafile))
	}

	return artifact.NewPackageArtifactFromYaml(content)
}

func (b *BackendLocal) GetExtraMetadata(file string) (*artifact.ExtraMetadata, error) {
	emetafile := filepath.Join(b.Path, file)
	fd, err := os.Open(emetafile)
	if err != nil {
		return nil, fmt.Errorf("cannot open " + emetafile)
	}
	defer fd.Close()

	bufferedReader := bufio.NewReader(fd)
	r, err := zstd.NewReader(bufferedReader)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var metadata artifact.ExtraMetadata
	if err := json.NewDecoder(r).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("error on decode extra metadata from file %s", file)
	}

	return &metadata, nil
}

func (b *BackendLocal) CleanFile(file string) error {
	absFile := filepath.Join(b.Path, file)
	return os.Remove(absFile)
}
