/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package specs

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

func (f *Finalizer) Yaml() ([]byte, error) {
	return yaml.Marshal(f)
}

func (f *Finalizer) WriteFinalize(path string) error {
	data, err := f.Yaml()
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

func (f *Finalizer) IsValid() bool {
	if len(f.Install) > 0 || len(f.Uninstall) > 0 {
		return true
	}
	return false
}
