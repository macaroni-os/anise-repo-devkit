/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package backends

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/geaaru/luet/pkg/logger"
	artifact "github.com/geaaru/luet/pkg/v2/compiler/types/artifact"
	"github.com/macaroni-os/anise-repo-devkit/pkg/specs"
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

func (b *BackendLocal) CleanFile(file string) error {
	absFile := filepath.Join(b.Path, file)
	return os.Remove(absFile)
}
