/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/

package main

import (
	"fmt"
	"os"
	"strings"

	devkitcmd "github.com/macaroni-os/anise-repo-devkit/pkg/cmd"
	"github.com/macaroni-os/anise-repo-devkit/pkg/devkit"

	anise_cfg "github.com/macaroni-os/anise/pkg/config"
	. "github.com/macaroni-os/anise/pkg/logger"
	"github.com/spf13/cobra"
)

const (
	cliName = `Copyright (c) 2020-2026 - Daniele Rondina

Anise Repository Devkit`
)

func initConfig() error {
	anise_cfg.AniseCfg.Viper.SetEnvPrefix("LUET")
	anise_cfg.AniseCfg.Viper.AutomaticEnv() // read in environment variables that match

	// Create EnvKey Replacer for handle complex structure
	replacer := strings.NewReplacer(".", "__")
	anise_cfg.AniseCfg.Viper.SetEnvKeyReplacer(replacer)
	anise_cfg.AniseCfg.Viper.SetTypeByDefaultValue(true)

	err := anise_cfg.AniseCfg.Viper.Unmarshal(&anise_cfg.AniseCfg)
	if err != nil {
		return err
	}

	InitAurora()
	NewSpinner()

	return nil
}

func Execute() {
	var rootCmd = &cobra.Command{
		Use:   "anise-repo-devkit --",
		Short: cliName,
		Version: fmt.Sprintf("%s-g%s %s - %s",
			devkit.Version, devkit.BuildCommit,
			devkit.BuildTime, devkit.BuildGoVersion,
		),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			err := initConfig()
			if err != nil {
				fmt.Println("Error on setup anise config/logger: " + err.Error())
				os.Exit(1)
			}

			debug, _ := cmd.Flags().GetBool("debug")
			if debug {
				anise_cfg.AniseCfg.GetGeneral().Debug = true
			}

		},
	}

	rootCmd.PersistentFlags().StringArrayP("tree", "t", []string{}, "Path of the tree to use.")
	rootCmd.PersistentFlags().StringP("specs-file", "s", "", "Path of the devkit specification file.")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logging.")

	rootCmd.AddCommand(
		devkitcmd.NewCleanCommand(),
		devkitcmd.NewPkgsCommand(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	Execute()
}
