/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package cmd

import (
	"fmt"
	"os"

	devkit "github.com/macaroni-os/anise-repo-devkit/pkg/devkit"
	specs "github.com/macaroni-os/anise-repo-devkit/pkg/specs"

	cobra "github.com/spf13/cobra"
)

func NewCleanCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "clean [OPTIONS]",
		Short: "Clean repository files.",
		PreRun: func(cmd *cobra.Command, args []string) {
			treePath, _ := cmd.Flags().GetStringArray("tree")

			if len(treePath) == 0 {
				fmt.Println("At least one tree path is needed.")
				os.Exit(1)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			var s *specs.AniseRDConfig
			var err error

			specsFile, _ := cmd.Flags().GetString("specs-file")
			backend, _ := cmd.Flags().GetString("backend")
			path, _ := cmd.Flags().GetString("path")
			treePath, _ := cmd.Flags().GetStringArray("tree")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			quiet, _ := cmd.Flags().GetBool("quiet")
			mottainaiProfile, _ := cmd.Flags().GetString("mottainai-profile")
			mottainaiMaster, _ := cmd.Flags().GetString("mottainai-master")
			mottainaiApiKey, _ := cmd.Flags().GetString("mottainai-apikey")
			mottainaiNamespace, _ := cmd.Flags().GetString("mottainai-namespace")

			minioBucket, _ := cmd.Flags().GetString("minio-bucket")
			minioAccessId, _ := cmd.Flags().GetString("minio-keyid")
			minioSecret, _ := cmd.Flags().GetString("minio-secret")
			minioEndpoint, _ := cmd.Flags().GetString("minio-endpoint")
			minioRegion, _ := cmd.Flags().GetString("minio-region")

			if specsFile == "" {
				s = specs.NewAniseRDConfig()
			} else {
				s, err = specs.LoadSpecsFile(specsFile)
				if err != nil {
					fmt.Println("Error on load specs: " + err.Error())
					os.Exit(1)
				}
			}

			opts := make(map[string]string, 0)
			if backend == "mottainai" {
				if mottainaiProfile != "" {
					opts["mottainai-profile"] = mottainaiProfile
				}
				if mottainaiMaster != "" {
					opts["mottainai-master"] = mottainaiMaster
				}
				if mottainaiApiKey != "" {
					opts["mottainai-apikey"] = mottainaiApiKey
				}
				if mottainaiNamespace != "" {
					opts["mottainai-namespace"] = mottainaiNamespace
				}
			} else if backend == "minio" {

				if minioEndpoint != "" {
					opts["minio-endpoint"] = minioEndpoint
				} else {
					opts["minio-endpoint"] = os.Getenv("MINIO_URL")
				}

				if minioBucket != "" {
					opts["minio-bucket"] = minioBucket
				} else {
					opts["minio-bucket"] = os.Getenv("MINIO_BUCKET")
				}

				if minioAccessId != "" {
					opts["minio-keyid"] = minioAccessId
				} else {
					opts["minio-keyid"] = os.Getenv("MINIO_ID")
				}

				if minioSecret != "" {
					opts["minio-secret"] = minioSecret
				} else {
					opts["minio-secret"] = os.Getenv("MINIO_SECRET")
				}

				opts["minio-region"] = minioRegion

			}

			repoCleaner, err := devkit.NewRepoCleaner(s, backend, path, opts, dryRun)
			if err != nil {
				fmt.Println("Error on initialize repo cleaner: " + err.Error())
				os.Exit(1)
			}

			if !quiet {
				repoCleaner.Verbose = true
			}

			// Loading tree in memory
			err = repoCleaner.LoadTrees(treePath)
			if err != nil {
				fmt.Println("Erro on loading trees: " + err.Error())
				os.Exit(1)
			}

			err = repoCleaner.Run()
			if err != nil {
				fmt.Println("Error on clean repository: " + err.Error())
				os.Exit(1)
			}

			if dryRun {
				fmt.Println(fmt.Sprintf(
					"All done. Processed file %d. Removable files %d.",
					repoCleaner.ProcessedFiles,
					len(repoCleaner.Files2Remove),
				))
			} else {
				fmt.Println(fmt.Sprintf(
					"All done. Processed file %d. Removed files %d.",
					repoCleaner.ProcessedFiles,
					len(repoCleaner.Files2Remove),
				))
			}
		},
	}

	var flags = cmd.Flags()
	flags.StringP("backend", "b", "local", "Select backend repository: local|mottainai.")
	flags.StringP("path", "p", "", "Path of the repository artefacts.")
	flags.Bool("dry-run", false, "Only check files to remove.")
	flags.Bool("quiet", false, "Quiet output.")
	flags.String("mottainai-profile", "", "Set mottainai profile to use.")
	flags.String("mottainai-master", "", "Set mottainai Server to use.")
	flags.String("mottainai-apikey", "", "Set mottainai API Key to use.")
	flags.String("mottainai-namespace", "", "Set mottainai namespace to use.")

	// Minio options
	flags.String("minio-bucket", "",
		"Set minio bucket to use or set env MINIO_BUCKET.")
	flags.String("minio-endpoint", "",
		"Set minio endpoint to use or set env MINIO_URL.")
	flags.String("minio-keyid", "",
		"Set minio Access Key to use or set env MINIO_ID.")
	flags.String("minio-secret", "",
		"Set minio Access Key to use or set env MINIO_SECRET.")
	flags.String("minio-region", "", "Optinally define the minio region.")

	return cmd
}
