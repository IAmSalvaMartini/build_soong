// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"

	"android/soong/zip"
)

type byteReaderCloser struct {
	*bytes.Reader
	io.Closer
}

type pathMapping struct {
	dest, src string
	zipMethod uint16
}

type uniqueSet map[string]bool

func (u *uniqueSet) String() string {
	return `""`
}

func (u *uniqueSet) Set(s string) error {
	if _, found := (*u)[s]; found {
		return fmt.Errorf("File %q was specified twice as a file to not deflate", s)
	} else {
		(*u)[s] = true
	}

	return nil
}

type file struct{}

type listFiles struct{}

type dir struct{}

func (f *file) String() string {
	return `""`
}

func (f *file) Set(s string) error {
	if relativeRoot == "" && !junkPaths {
		return fmt.Errorf("must pass -C or -j before -f")
	}

	fArgs = append(fArgs, zip.FileArg{
		PathPrefixInZip:     *rootPrefix,
		SourcePrefixToStrip: relativeRoot,
		JunkPaths:           junkPaths,
		SourceFiles:         []string{s},
	})

	return nil
}

func (l *listFiles) String() string {
	return `""`
}

func (l *listFiles) Set(s string) error {
	if relativeRoot == "" && !junkPaths {
		return fmt.Errorf("must pass -C or -j before -l")
	}

	list, err := ioutil.ReadFile(s)
	if err != nil {
		return err
	}

	fArgs = append(fArgs, zip.FileArg{
		PathPrefixInZip:     *rootPrefix,
		SourcePrefixToStrip: relativeRoot,
		JunkPaths:           junkPaths,
		SourceFiles:         strings.Split(string(list), "\n"),
	})

	return nil
}

func (d *dir) String() string {
	return `""`
}

func (d *dir) Set(s string) error {
	if relativeRoot == "" && !junkPaths {
		return fmt.Errorf("must pass -C or -j before -D")
	}

	fArgs = append(fArgs, zip.FileArg{
		PathPrefixInZip:     *rootPrefix,
		SourcePrefixToStrip: relativeRoot,
		JunkPaths:           junkPaths,
		GlobDir:             s,
	})

	return nil
}

type relativeRootImpl struct{}

func (*relativeRootImpl) String() string { return relativeRoot }

func (*relativeRootImpl) Set(s string) error {
	relativeRoot = s
	junkPaths = false
	return nil
}

type junkPathsImpl struct{}

func (*junkPathsImpl) IsBoolFlag() bool { return true }

func (*junkPathsImpl) String() string { return relativeRoot }

func (*junkPathsImpl) Set(s string) error {
	var err error
	junkPaths, err = strconv.ParseBool(s)
	relativeRoot = ""
	return err
}

var (
	rootPrefix   *string
	relativeRoot string
	junkPaths    bool

	fArgs            zip.FileArgs
	nonDeflatedFiles = make(uniqueSet)
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: zip -o zipfile [-m manifest] -C dir [-f|-l file]...\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	var expandedArgs []string
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "@") {
			bytes, err := ioutil.ReadFile(strings.TrimPrefix(arg, "@"))
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
			respArgs := zip.ReadRespFile(bytes)
			expandedArgs = append(expandedArgs, respArgs...)
		} else {
			expandedArgs = append(expandedArgs, arg)
		}
	}

	flags := flag.NewFlagSet("flags", flag.ExitOnError)

	out := flags.String("o", "", "file to write zip file to")
	manifest := flags.String("m", "", "input jar manifest file name")
	directories := flags.Bool("d", false, "include directories in zip")
	rootPrefix = flags.String("P", "", "path prefix within the zip at which to place files")
	compLevel := flags.Int("L", 5, "deflate compression level (0-9)")
	emulateJar := flags.Bool("jar", false, "modify the resultant .zip to emulate the output of 'jar'")
	writeIfChanged := flags.Bool("write_if_changed", false, "only update resultant .zip if it has changed")

	parallelJobs := flags.Int("parallel", runtime.NumCPU(), "number of parallel threads to use")
	cpuProfile := flags.String("cpuprofile", "", "write cpu profile to file")
	traceFile := flags.String("trace", "", "write trace to file")

	flags.Var(&listFiles{}, "l", "file containing list of .class files")
	flags.Var(&dir{}, "D", "directory to include in zip")
	flags.Var(&file{}, "f", "file to include in zip")
	flags.Var(&nonDeflatedFiles, "s", "file path to be stored within the zip without compression")
	flags.Var(&relativeRootImpl{}, "C", "path to use as relative root of files in following -f, -l, or -D arguments")
	flags.Var(&junkPathsImpl{}, "j", "junk paths, zip files without directory names")

	flags.Parse(expandedArgs[1:])

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments %s\n", strings.Join(flags.Args(), " "))
		usage()
	}

	err := zip.Run(zip.ZipArgs{
		FileArgs:                 fArgs,
		OutputFilePath:           *out,
		CpuProfileFilePath:       *cpuProfile,
		TraceFilePath:            *traceFile,
		EmulateJar:               *emulateJar,
		AddDirectoryEntriesToZip: *directories,
		CompressionLevel:         *compLevel,
		ManifestSourcePath:       *manifest,
		NumParallelJobs:          *parallelJobs,
		NonDeflatedFiles:         nonDeflatedFiles,
		WriteIfChanged:           *writeIfChanged,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
