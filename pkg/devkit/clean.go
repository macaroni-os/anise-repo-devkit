/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package devkit

import (
	"fmt"

	specs "github.com/macaroni-os/anise-repo-devkit/pkg/specs"

	. "github.com/geaaru/luet/pkg/logger"
)

type RepoCleaner struct {
	*RepoKnife
	DryRun bool
}

func NewRepoCleaner(s *specs.AniseRDConfig,
	backend, path string, opts map[string]string,
	dryRun bool) (*RepoCleaner, error) {

	knife, err := NewRepoKnife(s, backend, path, opts)
	if err != nil {
		return nil, err
	}

	ans := &RepoCleaner{
		RepoKnife: knife,
		DryRun:    dryRun,
	}

	return ans, nil
}

func (c *RepoCleaner) Run() error {

	err := c.RepoKnife.Analyze()

	if len(c.Files2Remove) > 0 {
		for _, f := range c.Files2Remove {
			if c.DryRun {
				InfoC(fmt.Sprintf("[%s] Could be removed.", f))
			} else {
				err = c.BackendHandler.CleanFile(f)
				if err != nil {
					Error(fmt.Sprintf("[%s] Error on removing file: %s", f, err.Error()))
				} else {
					InfoC(fmt.Sprintf("[%s] Removed.", f))
				}
			}
		}
	} else {
		InfoC("No files to remove.")
	}

	return nil
}
