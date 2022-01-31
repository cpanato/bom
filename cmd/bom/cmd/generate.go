/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"sigs.k8s.io/bom/pkg/spdx"
	"sigs.k8s.io/release-utils/util"
)

var genOpts = &generateOptions{}

var generateCmd = &cobra.Command{
	Short: "bom generate → Create SPDX SBOMs",
	Long: `bom generate → Create SPDX SBOMs

generate is the bom subcommand to generate SPDX manifests.

Currently supports creating SBOM from files, images, and docker
archives (images in tarballs). It supports pulling images from
remote registries for analysis.

bom can take a deeper look into images using a growing number
of analyzers designed to add more sense to common base images.

The SBOM data can also be exported to an in-toto provenance
attestation. The output will produce a provenance statement listing all
the SPDX data as in-toto subjects, but otherwise ready to be
completed by a later stage in your CI/CD pipeline. See the
--provenance flag for more details.

`,
	Use:               "generate",
	SilenceUsage:      true,
	SilenceErrors:     true,
	PersistentPreRunE: initLogging,
	RunE: func(cmd *cobra.Command, args []string) error {
		for i, arg := range args {
			if util.Exists(arg) {
				file, err := os.Open(arg)
				if err != nil {
					return errors.Wrapf(err, "checking argument %d", i)
				}
				fileInfo, err := file.Stat()
				if err != nil {
					return errors.Wrapf(err, "calling stat on argument %d", i)
				}
				if fileInfo.IsDir() {
					genOpts.directories = append(genOpts.directories, arg)
				}
			}
		}

		if err := genOpts.Validate(); err != nil {
			cmd.Help() // nolint:errcheck // We already errored
			return errors.Wrap(err, "validating command line options")
		}

		return generateBOM(genOpts)
	},
}

type generateOptions struct {
	analyze        bool
	noGitignore    bool
	noGoModules    bool
	noGoTransient  bool
	scanImages     bool
	name           string // Name to use in the document
	namespace      string
	outputFile     string
	configFile     string
	license        string
	provenancePath string // Path to export the SBOM as provenance statement
	workDir        string
	images         []string
	imageArchives  []string
	archives       []string
	files          []string
	directories    []string
	ignorePatterns []string
}

// Validate verify options consistency
func (opts *generateOptions) Validate() error {
	if opts.configFile == "" &&
		len(opts.images) == 0 &&
		len(opts.files) == 0 &&
		len(opts.imageArchives) == 0 &&
		len(opts.archives) == 0 &&
		len(opts.archives) == 0 &&
		len(opts.directories) == 0 {
		return errors.New("to generate a SPDX BOM you have to provide at least one image or file")
	}

	// Check if specified local files exist
	for _, col := range []struct {
		Items []string
		Name  string
	}{
		{opts.imageArchives, "image archive"},
		{opts.files, "file"},
		{opts.directories, "directory"},
		{opts.archives, "archive"},
	} {
		// Check if image archives exist
		for i, iPath := range col.Items {
			if !util.Exists(iPath) {
				return errors.Errorf("%s #%d not found (%s)", col.Name, i+1, iPath)
			}
		}
	}

	if opts.workDir != "" {
		if _, err := os.Stat(opts.workDir); os.IsNotExist(err) {
			return errors.Errorf("directory %s not found", opts.workDir)
		}
	}

	return nil
}

func init() {
	generateCmd.PersistentFlags().StringSliceVarP(
		&genOpts.images,
		"image",
		"i",
		[]string{},
		"list of images",
	)

	generateCmd.PersistentFlags().StringSliceVarP(
		&genOpts.files,
		"file",
		"f",
		[]string{},
		"list of files to include",
	)

	generateCmd.PersistentFlags().StringVarP(
		&genOpts.workDir,
		"workDir",
		"C",
		"",
		"Base working directory",
	)

	generateCmd.PersistentFlags().StringSliceVarP(
		&genOpts.imageArchives,
		"tarball",
		"t",
		[]string{},
		"list of docker archive tarballs to include in the manifest",
	)

	if err := generateCmd.PersistentFlags().MarkDeprecated(
		"tarball", "tarball has been renamed to image-archive",
	); err != nil {
		logrus.Fatal(errors.Wrap(err, "marking flag as deprecated"))
	}

	generateCmd.PersistentFlags().StringSliceVar(
		&genOpts.imageArchives,
		"image-archive",
		[]string{},
		"list of docker archive tarballs to include in the manifest",
	)

	generateCmd.PersistentFlags().StringSliceVar(
		&genOpts.archives,
		"archive",
		[]string{},
		"list of archives to add as packages (supports tar, tar.gz)",
	)

	generateCmd.PersistentFlags().StringSliceVarP(
		&genOpts.directories,
		"dirs",
		"d",
		[]string{},
		"list of directories to include in the manifest as packages",
	)

	generateCmd.PersistentFlags().StringSliceVar(
		&genOpts.ignorePatterns,
		"ignore",
		[]string{},
		"list of regexp patterns to ignore when scanning directories",
	)

	generateCmd.PersistentFlags().StringVarP(
		&genOpts.license,
		"license",
		"l",
		"",
		"SPDX license identifier to declare in the SBOM",
	)

	generateCmd.PersistentFlags().BoolVar(
		&genOpts.noGitignore,
		"no-gitignore",
		false,
		"don't use exclusions from .gitignore files",
	)

	generateCmd.PersistentFlags().BoolVar(
		&genOpts.noGoModules,
		"no-gomod",
		false,
		"don't perform go.mod analysis, sbom will not include data about go packages",
	)

	generateCmd.PersistentFlags().BoolVar(
		&genOpts.noGoTransient,
		"no-transient",
		false,
		"don't include transient go dependencies, only direct deps from go.mod",
	)

	generateCmd.PersistentFlags().StringVarP(
		&genOpts.namespace,
		"namespace",
		"n",
		"",
		"an URI that servers as namespace for the SPDX doc",
	)

	generateCmd.PersistentFlags().StringVarP(
		&genOpts.outputFile,
		"output",
		"o",
		"",
		"path to the file where the document will be written (defaults to STDOUT)",
	)

	generateCmd.PersistentFlags().BoolVarP(
		&genOpts.analyze,
		"analyze-images",
		"a",
		false,
		"go deeper into images using the available analyzers",
	)

	generateCmd.PersistentFlags().StringVarP(
		&genOpts.configFile,
		"config",
		"c",
		"",
		"path to yaml SBOM configuration file",
	)

	generateCmd.PersistentFlags().StringVar(
		&genOpts.provenancePath,
		"provenance",
		"",
		"path to export the SBOM as an in-toto provenance statement",
	)

	generateCmd.PersistentFlags().BoolVar(
		&genOpts.scanImages,
		"scan-images",
		true,
		"scan container images to look for OS information (currently debian only)",
	)

	generateCmd.PersistentFlags().StringVar(
		&genOpts.name,
		"name",
		"",
		"name for the document, in contrast to URLs, intended for humans",
	)

	if err := generateCmd.MarkPersistentFlagDirname("dirs"); err != nil {
		logrus.Error("error marking flag as directory")
	}
	for _, fl := range []string{"config", "image-archive", "file", "archive"} {
		if err := generateCmd.MarkPersistentFlagFilename(fl); err != nil {
			logrus.Error("error marking flag as file")
		}
	}
}

func generateBOM(opts *generateOptions) error {
	logrus.Info("Generating SPDX Bill of Materials")

	builder := spdx.NewDocBuilder()
	builderOpts := &spdx.DocGenerateOptions{
		Tarballs:         opts.imageArchives,
		Archives:         opts.archives,
		Files:            opts.files,
		Images:           opts.images,
		Directories:      opts.directories,
		OutputFile:       opts.outputFile,
		Namespace:        opts.namespace,
		AnalyseLayers:    opts.analyze,
		ProcessGoModules: !opts.noGoModules,
		OnlyDirectDeps:   !opts.noGoTransient,
		ConfigFile:       opts.configFile,
		License:          opts.license,
		ScanImages:       opts.scanImages,
		Name:             opts.name,
		WorkDir:          opts.workDir,
	}

	// We only replace the ignore patterns one or more where defined
	if len(opts.ignorePatterns) > 0 {
		builderOpts.IgnorePatterns = opts.ignorePatterns
	}
	doc, err := builder.Generate(builderOpts)
	if err != nil {
		return errors.Wrap(err, "generating doc")
	}

	if opts.outputFile == "" {
		markup, err := doc.Render()
		if err != nil {
			return errors.Wrap(err, "rendering document")
		}
		fmt.Println(markup)
	}

	// Export the SBOM as in-toto provenance
	if opts.provenancePath != "" {
		if err := doc.WriteProvenanceStatement(
			spdx.DefaultProvenanceOptions, opts.provenancePath,
		); err != nil {
			return errors.Wrap(err, "writing SBOM as provenance statement")
		}
	}

	return nil
}
